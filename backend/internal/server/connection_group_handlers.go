package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

type connectionInstanceGroupNameRequest struct {
	Name     string  `json:"name"`
	Revision *uint64 `json:"revision,omitempty"`
}

type connectionInstanceGroupRevisionRequest struct {
	Revision *uint64 `json:"revision"`
}

func (s *Server) listConnectionInstanceGroups(w http.ResponseWriter, _ *http.Request, sessionID string) {
	writeJSON(w, http.StatusOK, map[string]any{"layout": s.connectionInstanceLayout(sessionID)})
}

func (s *Server) createConnectionInstanceGroup(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body connectionInstanceGroupNameRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	layout := s.connectionInstanceLayout(sessionID)
	if body.Revision != nil && *body.Revision != layout.Revision {
		writeLayoutConflict(w, layout)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, "connection instance group name is required", "name")
		return
	}
	for _, group := range layout.Groups {
		if strings.EqualFold(group.Name, name) {
			writeConnectionInstanceGroupError(w, http.StatusBadRequest, "connection instance group name already exists", "name")
			return
		}
	}
	groupID, err := persistence.NewUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	next := cloneConnectionInstanceLayout(layout)
	next.Groups = append(next.Groups, persistence.ConnectionInstanceGroup{
		GroupID:               groupID,
		Name:                  name,
		ConnectionInstanceIDs: []string{},
	})
	ungroupedIndex := len(next.GroupOrder)
	for index, id := range next.GroupOrder {
		if id == persistence.UngroupedConnectionInstanceGroupID {
			ungroupedIndex = index
			break
		}
	}
	next.GroupOrder = append(next.GroupOrder, "")
	copy(next.GroupOrder[ungroupedIndex+1:], next.GroupOrder[ungroupedIndex:])
	next.GroupOrder[ungroupedIndex] = groupID
	if err := s.saveConnectionInstanceLayout(w, sessionID, layout, next); err != nil {
		return
	}
	next.Revision = layout.Revision + 1
	if next.Revision == 0 {
		next.Revision = 1
	}
	writeJSON(w, http.StatusCreated, map[string]any{"layout": next})
}

func (s *Server) renameConnectionInstanceGroup(w http.ResponseWriter, r *http.Request, sessionID string) {
	groupID := r.PathValue("groupId")
	if groupID == persistence.UngroupedConnectionInstanceGroupID {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, "ungrouped cannot be renamed", "groupId")
		return
	}
	var body connectionInstanceGroupNameRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Revision == nil {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, "revision is required", "revision")
		return
	}
	layout := s.connectionInstanceLayout(sessionID)
	if *body.Revision != layout.Revision {
		writeLayoutConflict(w, layout)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, "connection instance group name is required", "name")
		return
	}
	found := false
	next := cloneConnectionInstanceLayout(layout)
	for index := range next.Groups {
		if next.Groups[index].GroupID == groupID {
			found = true
			next.Groups[index].Name = name
		}
		if next.Groups[index].GroupID != groupID && strings.EqualFold(next.Groups[index].Name, name) {
			writeConnectionInstanceGroupError(w, http.StatusBadRequest, "connection instance group name already exists", "name")
			return
		}
	}
	if !found {
		writeConnectionInstanceGroupError(w, http.StatusNotFound, "connection instance group not found", "groupId")
		return
	}
	if err := s.saveConnectionInstanceLayout(w, sessionID, layout, next); err != nil {
		return
	}
	next.Revision = layout.Revision + 1
	if next.Revision == 0 {
		next.Revision = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"layout": next})
}

func (s *Server) deleteConnectionInstanceGroup(w http.ResponseWriter, r *http.Request, sessionID string) {
	groupID := r.PathValue("groupId")
	if groupID == persistence.UngroupedConnectionInstanceGroupID {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, "ungrouped cannot be deleted", "groupId")
		return
	}
	var body connectionInstanceGroupRevisionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Revision == nil {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, "revision is required", "revision")
		return
	}
	layout := s.connectionInstanceLayout(sessionID)
	if *body.Revision != layout.Revision {
		writeLayoutConflict(w, layout)
		return
	}
	next := cloneConnectionInstanceLayout(layout)
	for index, group := range next.Groups {
		if group.GroupID != groupID {
			continue
		}
		if len(group.ConnectionInstanceIDs) > 0 {
			writeConnectionInstanceGroupError(w, http.StatusConflict, "connection instance group is not empty", "groupId", map[string]any{"layout": layout})
			return
		}
		next.Groups = append(next.Groups[:index], next.Groups[index+1:]...)
		next.GroupOrder = removeGroupID(next.GroupOrder, groupID)
		if err := s.saveConnectionInstanceLayout(w, sessionID, layout, next); err != nil {
			return
		}
		next.Revision = layout.Revision + 1
		if next.Revision == 0 {
			next.Revision = 1
		}
		writeJSON(w, http.StatusOK, map[string]any{"layout": next})
		return
	}
	writeConnectionInstanceGroupError(w, http.StatusNotFound, "connection instance group not found", "groupId")
}

func (s *Server) replaceConnectionInstanceLayout(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body persistence.ConnectionInstanceLayout
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	layout := s.connectionInstanceLayout(sessionID)
	if body.Revision != layout.Revision {
		writeLayoutConflict(w, layout)
		return
	}
	if hasFullConnectionInstanceGroup(body) {
		writeConnectionInstanceGroupError(w, http.StatusConflict, "connection instance group limit reached (10)", "layout", map[string]any{"layout": layout})
		return
	}
	if err := persistence.ValidateConnectionInstanceLayout(&body); err != nil {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, err.Error())
		return
	}
	instances := s.connectionInstanceSummaries()
	next, _ := normalizeConnectionInstanceLayout(body, true, nil, instances)
	if err := s.saveConnectionInstanceLayout(w, sessionID, layout, next); err != nil {
		return
	}
	next.Revision = layout.Revision + 1
	if next.Revision == 0 {
		next.Revision = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"layout": next})
}

func (s *Server) saveConnectionInstanceLayout(w http.ResponseWriter, sessionID string, current, next persistence.ConnectionInstanceLayout) error {
	next.Revision = current.Revision + 1
	if next.Revision == 0 {
		next.Revision = 1
	}
	if hasFullConnectionInstanceGroup(next) {
		writeConnectionInstanceGroupError(w, http.StatusConflict, "connection instance group limit reached (10)", "layout", map[string]any{"layout": current})
		return errors.New("connection instance group exceeds maximum size")
	}
	if err := persistence.ValidateConnectionInstanceLayout(&next); err != nil {
		writeConnectionInstanceGroupError(w, http.StatusBadRequest, err.Error())
		return err
	}
	if err := s.auth.SetConnectionInstanceLayout(sessionID, next); err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return err
	}
	return nil
}

func hasFullConnectionInstanceGroup(layout persistence.ConnectionInstanceLayout) bool {
	for _, group := range layout.Groups {
		if len(group.ConnectionInstanceIDs) > 10 {
			return true
		}
	}
	return false
}

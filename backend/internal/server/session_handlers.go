package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

type createConnectionRequest struct {
	ConnectionDefinitionID        string  `json:"connectionDefinitionId"`
	Cols                          int     `json:"cols,omitempty"`
	Rows                          int     `json:"rows,omitempty"`
	InitialCwd                    *string `json:"initialCwd,omitempty"`
	ReuseFromConnectionInstanceID *string `json:"reuseFromConnectionInstanceId,omitempty"`
}

func (s *Server) listConnectionInstances(w http.ResponseWriter, _ *http.Request, _ string) {
	writeJSON(w, http.StatusOK, map[string]any{"connectionInstances": s.terms.Summaries()})
}

func (s *Server) getConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("connectionInstanceId")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	for _, item := range s.terms.Summaries() {
		if item.ID == id {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) createConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	var body createConnectionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	cwd := ""
	if body.InitialCwd != nil {
		cwd = *body.InitialCwd
	}
	var result terminal.Summary
	var err error
	if body.ConnectionDefinitionID != "" && body.ConnectionDefinitionID != "local" {
		result, err = s.terms.CreateRemote(r.Context(), body.ConnectionDefinitionID, body.Cols, body.Rows, stringValue(body.ReuseFromConnectionInstanceID))
	} else {
		result, err = s.terms.Create(r.Context(), cwd, body.Cols, body.Rows)
	}
	if err != nil {
		if errors.Is(err, connection.ErrTransportDraining) {
			writeError(w, http.StatusConflict, "ssh transport is draining", "transport")
		} else if errors.Is(err, connection.ErrTransportUnavailable) {
			writeError(w, http.StatusConflict, "ssh transport unavailable", "transport")
		} else if errors.Is(err, connection.ErrConnectionCapacity) {
			writeError(w, http.StatusConflict, "connection capacity reached", "capacity")
		} else {
			writeError(w, http.StatusBadRequest, err.Error(), "connection")
		}
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) createConnectionLaunch(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body createConnectionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.ConnectionDefinitionID == "" || body.ConnectionDefinitionID == "local" {
		writeError(w, http.StatusBadRequest, "tmux launches require an SSH definition")
		return
	}
	result, err := s.terms.CreateRemoteLaunchOwned(r.Context(), body.ConnectionDefinitionID, body.Cols, body.Rows, stringValue(body.ReuseFromConnectionInstanceID), sessionID)
	if err != nil {
		if errors.Is(err, connection.ErrTmuxNotEnabled) {
			writeError(w, http.StatusConflict, "tmux is not enabled for this connection", "tmux")
		} else if errors.Is(err, connection.ErrTransportDraining) {
			writeError(w, http.StatusConflict, "ssh transport is draining", "transport")
		} else if errors.Is(err, connection.ErrTransportUnavailable) {
			writeError(w, http.StatusConflict, "ssh transport unavailable", "transport")
		} else if errors.Is(err, connection.ErrConnectionCapacity) {
			writeError(w, http.StatusConflict, "connection capacity reached", "capacity")
		} else {
			writeError(w, http.StatusBadRequest, err.Error(), "connection")
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"launchId": result.ID, "connectionDefinitionId": result.ConnectionDefinitionID, "lifecycle": result.Lifecycle, "tmuxSessionName": result.TmuxSessionName})
}

func (s *Server) deleteConnectionLaunch(w http.ResponseWriter, r *http.Request, sessionID string) {
	id := r.PathValue("launchId")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if owner := s.terms.PendingOwner(id); owner != "" && owner != sessionID {
		writeError(w, http.StatusForbidden, "launch belongs to another auth session")
		return
	}
	if err := s.terms.AbortRemoteLaunch(r.Context(), id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Server) deleteConnectionInstance(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("connectionInstanceId")
	if id == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.terms.Delete(r.Context(), id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateConnectionTitleRequest struct {
	Title json.RawMessage `json:"title"`
}

func (s *Server) updateConnectionTitle(w http.ResponseWriter, r *http.Request, _ string) {
	var body updateConnectionTitleRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if len(body.Title) == 0 {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	var title *string
	if strings.TrimSpace(string(body.Title)) != "null" {
		var value string
		if err := json.Unmarshal(body.Title, &value); err != nil {
			writeError(w, http.StatusBadRequest, "title must be a string or null")
			return
		}
		title = &value
	}
	id := r.PathValue("connectionInstanceId")
	result, err := s.terms.SetTitle(id, title)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "not found")
		} else if errors.Is(err, os.ErrProcessDone) {
			writeError(w, http.StatusConflict, "terminal is closed")
		} else if strings.Contains(err.Error(), "title") {
			writeError(w, http.StatusBadRequest, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

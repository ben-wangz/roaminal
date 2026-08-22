package server

import (
	"errors"
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

type reorderConnectionInstancesRequest struct {
	ConnectionInstanceIDs []string `json:"connectionInstanceIds"`
}

func (s *Server) reorderConnectionInstances(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body reorderConnectionInstancesRequest
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	instances := s.connectionInstanceSummaries()
	order, err := normalizeConnectionInstanceOrder(body.ConnectionInstanceIDs, instances)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection instance order", "connectionInstanceIds")
		return
	}
	layout := s.connectionInstanceLayout(sessionID)
	if hasUserConnectionInstanceGroups(layout) {
		writeError(w, http.StatusConflict, "connection instance groups own the sidebar layout", "layout")
		return
	}
	layout.UngroupedConnectionInstanceIDs = order
	layout.Revision++
	if layout.Revision == 0 {
		layout.Revision = 1
	}
	if err := s.auth.SetConnectionInstanceLayout(sessionID, layout); err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	ordered := orderConnectionInstances(instances, order)
	if s.agent != nil {
		for index := range ordered {
			ordered[index].Agent = s.agent.Summary(ordered[index])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectionInstances": ordered, "connectionInstanceLayout": layout})
}

func (s *Server) orderedConnectionInstances(sessionID string) []connection.Summary {
	instances := s.connectionInstanceSummaries()
	layout := s.connectionInstanceLayout(sessionID)
	instances = orderConnectionInstances(instances, flattenConnectionInstanceLayout(layout))
	if s.agent == nil {
		return instances
	}
	for index := range instances {
		instances[index].Agent = s.agent.Summary(instances[index])
	}
	return instances
}

func normalizeConnectionInstanceOrder(order []string, instances []connection.Summary) ([]string, error) {
	if err := persistence.ValidateConnectionInstanceOrder(order); err != nil {
		return nil, err
	}
	available := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		available[instance.ID] = struct{}{}
	}
	normalized := make([]string, 0, len(instances))
	seen := make(map[string]struct{}, len(instances))
	for _, id := range order {
		if _, exists := available[id]; !exists {
			continue
		}
		normalized = append(normalized, id)
		seen[id] = struct{}{}
	}
	for _, instance := range instances {
		if _, exists := seen[instance.ID]; !exists {
			normalized = append(normalized, instance.ID)
		}
	}
	return normalized, nil
}

func orderConnectionInstances(instances []connection.Summary, order []string) []connection.Summary {
	if len(instances) < 2 || len(order) == 0 {
		return instances
	}
	byID := make(map[string]connection.Summary, len(instances))
	for _, instance := range instances {
		byID[instance.ID] = instance
	}
	result := make([]connection.Summary, 0, len(instances))
	seen := make(map[string]struct{}, len(instances))
	for _, id := range order {
		if instance, exists := byID[id]; exists {
			result = append(result, instance)
			seen[id] = struct{}{}
		}
	}
	for _, instance := range instances {
		if _, exists := seen[instance.ID]; !exists {
			result = append(result, instance)
		}
	}
	return result
}

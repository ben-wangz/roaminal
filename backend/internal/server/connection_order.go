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
	instances := s.terms.Summaries()
	order, err := normalizeConnectionInstanceOrder(body.ConnectionInstanceIDs, instances)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection instance order", "connectionInstanceIds")
		return
	}
	if err := s.auth.SetConnectionInstanceOrder(sessionID, order); err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectionInstances": orderConnectionInstances(instances, order)})
}

func (s *Server) orderedConnectionInstances(sessionID string) []connection.Summary {
	return orderConnectionInstances(s.terms.Summaries(), s.auth.ConnectionInstanceOrder(sessionID))
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

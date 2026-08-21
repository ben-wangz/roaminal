package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
)

func (s *Server) agentSummary(w http.ResponseWriter, r *http.Request, _ string) {
	if s.agent == nil || s.terms == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	id := r.PathValue("connectionInstanceId")
	for _, summary := range s.terms.Summaries() {
		if summary.ID == id {
			writeJSON(w, http.StatusOK, s.agent.Details(r.Context(), summary, strings.TrimSpace(r.Header.Get("Origin"))))
			return
		}
	}
	writeAgentError(w, &agent.Error{Code: "agent_instance_not_found", Status: 404, Message: "The connection instance was not found."})
}

func (s *Server) startAgentInitialization(w http.ResponseWriter, r *http.Request, _ string) {
	var body map[string]json.RawMessage
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body == nil || len(body) != 0 {
		writeAgentError(w, &agent.Error{Code: "agent_event_invalid", Status: 400, Message: "Initialization accepts an empty JSON object."})
		return
	}
	if s.agent == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	result, err := s.agent.StartInitialization(r.Context(), r.PathValue("connectionInstanceId"), strings.TrimSpace(r.Header.Get("Origin")))
	if err != nil {
		writeAgentError(w, err)
		return
	}
	status := http.StatusAccepted
	if result.Status == "completed" {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) getAgentInitialization(w http.ResponseWriter, r *http.Request, _ string) {
	if s.agent == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	result, ok := s.agent.GetInitialization(r.PathValue("initializationId"))
	if !ok {
		writeAgentError(w, &agent.Error{Code: "agent_instance_not_found", Status: 404, Message: "The initialization operation was not found."})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) agentEvent(w http.ResponseWriter, r *http.Request) {
	if s.agent == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeAgentError(w, &agent.Error{Code: "agent_event_invalid", Status: 400, Message: "Content-Type must be application/json."})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024+1))
	if err != nil {
		writeAgentError(w, &agent.Error{Code: "agent_event_invalid", Status: 400, Message: "The Agent event could not be read."})
		return
	}
	duplicate, err := s.agent.AcceptEvent(agentBearer(r), body)
	if err != nil {
		writeAgentError(w, err)
		return
	}
	if duplicate {
		writeJSON(w, http.StatusOK, map[string]string{"result": "duplicate"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"result": "accepted"})
}

func agentBearer(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func writeAgentError(w http.ResponseWriter, value error) {
	if typed, ok := value.(*agent.Error); ok {
		if typed.Code == "agent_event_rate_limited" {
			w.Header().Set("Retry-After", "60")
		}
		writeJSON(w, typed.Status, map[string]string{"error": typed.Message, "code": typed.Code})
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

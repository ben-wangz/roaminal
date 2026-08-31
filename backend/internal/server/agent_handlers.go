package server

import (
	"net/http"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
)

func (s *Server) agentSummary(w http.ResponseWriter, r *http.Request, _ string) {
	if s.agentProvisioning == nil || s.terms == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	id := r.PathValue("connectionInstanceId")
	for _, summary := range s.terms.Summaries() {
		if summary.ID == id {
			writeJSON(w, http.StatusOK, s.agentProvisioning.Details(r.Context(), agentConnectionInstanceView(summary)))
			return
		}
	}
	writeAgentError(w, &agent.Error{Code: "agent_instance_not_found", Status: 404, Message: "The connection instance was not found."})
}

func (s *Server) startAgentInitialization(w http.ResponseWriter, r *http.Request, _ string) {
	var body *struct{}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body == nil {
		writeAgentError(w, &agent.Error{Code: "agent_initialization_invalid", Status: 400, Message: "Initialization accepts an empty JSON object."})
		return
	}
	if s.agentProvisioning == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	result, err := s.agentProvisioning.StartInitialization(r.Context(), r.PathValue("connectionInstanceId"))
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
	if s.agentProvisioning == nil {
		writeAgentError(w, &agent.Error{Code: "agent_store_unavailable", Status: 503, Message: "Agent state storage is unavailable."})
		return
	}
	result, ok := s.agentProvisioning.GetInitialization(r.PathValue("initializationId"))
	if !ok {
		writeAgentError(w, &agent.Error{Code: "agent_instance_not_found", Status: 404, Message: "The initialization operation was not found."})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeAgentError(w http.ResponseWriter, value error) {
	if typed, ok := value.(*agent.Error); ok {
		writeCodedError(w, typed.Status, typed.Message, typed.Code, nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

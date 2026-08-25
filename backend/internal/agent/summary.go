package agent

import (
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) Summary(summary ports.ConnectionInstanceView) ports.AgentSummary {
	result := ports.AgentSummary{
		AgentType: "codex", Support: "unsupported", Component: "uninitialized",
		Activity: "unknown", ActivityLabel: "Codex status unknown",
	}
	if summary.Type != "ssh" {
		result.SupportReason = "local_connection"
		return result
	}
	if !summary.TmuxEnabled || summary.TmuxSessionName == "" {
		result.SupportReason = "tmux_disabled"
		return result
	}
	if summary.Lifecycle != "live" {
		result.SupportReason = "connection_not_live"
		return result
	}
	if s.terms == nil {
		result.SupportReason = "ssh_transport_unavailable"
		return result
	}
	if _, err := s.terms.RemoteTransferInfo(summary.ID); err != nil {
		result.SupportReason = "ssh_transport_unavailable"
		return result
	}
	result.Support = "supported"
	if s.store.Err() != nil {
		result.Component = "error"
		result.ErrorCode = "agent_store_unavailable"
		result.ErrorMessage = "Agent state storage is unavailable."
		return result
	}
	target, record, ok := s.targetFor(summary)
	if !ok {
		return result
	}
	result.Component = record.InstallationState
	if result.Component == "" {
		result.Component = "uninitialized"
	}
	result.ComponentVersion = record.ComponentVersion
	if state, exists := s.runtimeState(target); exists {
		result = s.summaryFromState(result, state)
	} else if state, exists := record.Targets[target.SessionName]; exists {
		result = s.summaryFromState(result, state)
	}
	if result.Component == "initializing" && !s.endpointOperationRunning(target.EndpointKey) {
		result.Component = "error"
		result.InitializationID = ""
		result.ErrorCode = "agent_initialization_interrupted"
		result.ErrorMessage = "The previous Agent initialization was interrupted and can be repaired."
	}
	return result
}

func (s *Service) runtimeState(target Target) (TargetState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.runtime[target.EndpointKey+"\x00"+target.SessionName]
	return state, ok
}

func (s *Service) endpointOperationRunning(endpointKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	operationID := s.endpointOps[endpointKey]
	operation := s.operations[operationID]
	return operation != nil && operation.Status == "running"
}

func (s *Service) summaryFromState(result ports.AgentSummary, state TargetState) ports.AgentSummary {
	result.Component = state.Component
	if result.Component == "" {
		result.Component = "uninitialized"
	}
	result.ComponentVersion = state.ComponentVersion
	result.Activity = state.Activity
	result.ActivityLabel = activityLabel(state.Activity)
	result.LastEventName = state.LastEventName
	if !state.LastEventAt.IsZero() {
		result.LastEventAt = state.LastEventAt.UTC().Format(time.RFC3339Nano)
	}
	result.InitializationID = state.InitializationID
	result.ErrorCode = state.ErrorCode
	result.ErrorMessage = state.ErrorMessage
	if stale := s.staleActivity(state); stale != "" {
		result.Activity = stale
		result.ActivityLabel = activityLabel(stale)
	}
	return result
}

func activityLabel(value string) string {
	switch value {
	case "running":
		return "Codex running"
	case "waiting":
		return "Codex waiting for permission"
	case "completed":
		return "Codex turn finished"
	case "idle":
		return "Codex idle"
	case "stale":
		return "Codex status stale"
	default:
		return "Codex status unknown"
	}
}

func (s *Service) staleActivity(state TargetState) string {
	when := state.LastReceivedAt
	if when.IsZero() {
		when = state.LastEventAt
	}
	if when.IsZero() {
		return ""
	}
	age := s.since(when)
	switch state.Activity {
	case "running", "waiting":
		if age > 2*time.Hour {
			return "stale"
		}
	case "completed":
		if age > 30*time.Minute {
			return "idle"
		}
	case "idle":
		if age > 24*time.Hour {
			return "stale"
		}
	}
	return ""
}

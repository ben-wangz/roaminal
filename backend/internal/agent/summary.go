package agent

import (
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) Summary(summary ports.ConnectionInstanceView) ports.AgentSummary {
	result := ports.AgentSummary{
		AgentType: "codex", Support: "unsupported", Component: "uninitialized",
		Activity: "unknown", ActivityLabel: "Codex status unknown", StateLabel: "Agent status unknown",
		SyncStatus: "unavailable",
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
		result.SupportReason = "connection_service_unavailable"
		return result
	}
	result.Support = "supported"
	if s.store.Err() != nil {
		result.Component = "error"
		result.ErrorCode = "agent_store_unavailable"
		result.ErrorMessage = "Agent state storage is unavailable."
		return result
	}
	_, record, ok := s.targetFor(summary)
	if !ok {
		return result
	}
	result.Component = record.InstallationState
	if result.Component == "" {
		result.Component = "uninitialized"
	}
	result.ComponentVersion = record.ComponentVersion
	if state, exists := record.Targets[summary.TmuxSessionName]; exists {
		result = s.summaryFromState(result, state)
	}
	if result.Component == "initializing" && !s.endpointOperationRunningForSummary(summary) {
		result.Component = "error"
		result.InitializationID = ""
		result.ErrorCode = "agent_initialization_interrupted"
		result.ErrorMessage = "The previous Agent initialization was interrupted and can be repaired."
	}
	return result
}

func (s *Service) endpointOperationRunningForSummary(summary ports.ConnectionInstanceView) bool {
	target, _, ok := s.targetFor(summary)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	operationID := s.endpointOps[target.EndpointKey]
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
	if state.Provider != "" {
		result.AgentType = state.Provider
	}
	result.State = state.State
	result.StateLabel = stateLabel(state.State)
	result.StateIndex = state.StateIndex
	if !state.StateUpdatedAt.IsZero() {
		result.StateUpdatedAt = state.StateUpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	result.SyncStatus = state.SyncStatus
	if result.SyncStatus == "" {
		result.SyncStatus = "unavailable"
	}
	if !state.LastSyncedAt.IsZero() {
		result.LastSyncedAt = state.LastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	result.SyncError = state.SyncError
	return result
}

func stateLabel(value string) string {
	switch value {
	case "running":
		return "Agent running"
	case "relax":
		return "Agent idle"
	case "error":
		return "Agent error"
	default:
		return "Agent status unknown"
	}
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

package agent

import (
	"errors"
	"fmt"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var errStaleSnapshot = errors.New("agent state snapshot is older than the cached projection")

func (s *Service) acceptSnapshot(endpointKey string, target Target, view ports.ConnectionInstanceView, _ EndpointRecord, snapshot remoteAgentState) error {
	lock := s.endpointMutex(endpointKey)
	lock.Lock()
	defer lock.Unlock()
	current, exists := s.store.Get(endpointKey)
	if !exists {
		current = EndpointRecord{Targets: map[string]TargetState{}}
	}
	previous := current.Targets[target.SessionName]
	sameRuntime := previous.RuntimeID != "" && previous.RuntimeID == snapshot.RuntimeID
	if previous.RuntimeID != "" && !sameRuntime && olderRuntime(previous, snapshot) {
		_ = s.recordSyncFailureForTarget(endpointKey, target, syncStatusStale, errStaleSnapshot)
		return errStaleSnapshot
	}
	if sameRuntime && snapshot.LatestIndex < previous.StateIndex {
		_ = s.recordSyncFailureForTarget(endpointKey, target, syncStatusStale, errStaleSnapshot)
		return errStaleSnapshot
	}
	if sameRuntime && snapshot.LatestIndex == previous.StateIndex {
		if previous.State != "" {
			if previous.State != snapshot.State {
				_ = s.recordSyncFailureForTarget(endpointKey, target, syncStatusStale, errStaleSnapshot)
				return errStaleSnapshot
			}
			return s.updateSyncMetadata(endpointKey, target, snapshot)
		}
	}

	transition := sameRuntime && previous.State != "" && previous.State != snapshot.State
	transitionKey := ""
	if transition {
		transitionKey = snapshot.RuntimeID + "\x00" + fmt.Sprint(snapshot.LatestIndex)
		if previous.LastTransitionKey != transitionKey {
			if err := s.appendTransitionMessage(view, endpointKey, target, current, previous.State, snapshot); err != nil {
				return err
			}
		}
	}
	return s.store.Update(endpointKey, func(record *EndpointRecord) error {
		value := record.Targets[target.SessionName]
		value.SessionName = target.SessionName
		value.Provider = snapshot.Provider
		value.SessionID = snapshot.Tmux.SessionID
		value.SessionCreated = snapshot.Tmux.SessionCreated
		value.SocketFingerprint = snapshot.Tmux.SocketFingerprint
		value.RuntimeID = snapshot.RuntimeID
		value.State = snapshot.State
		value.StateIndex = snapshot.LatestIndex
		value.StateUpdatedAt = snapshot.UpdatedAt.UTC()
		value.SyncStatus = syncStatusAvailable
		value.LastSyncedAt = s.now().UTC()
		value.SyncError = ""
		value.Component = "ready"
		value.ComponentVersion = snapshot.ComponentVersion
		value.Activity = legacyActivity(snapshot.State)
		value.LastEventAt = snapshot.Records[len(snapshot.Records)-1].Timestamp.UTC()
		value.LastEventName = snapshot.Records[len(snapshot.Records)-1].EventName
		if transitionKey != "" {
			value.LastTransitionKey = transitionKey
		}
		value.ErrorCode, value.ErrorMessage = "", ""
		record.Targets[target.SessionName] = value
		record.ComponentVersion = snapshot.ComponentVersion
		if record.InstallationState == "" || record.InstallationState == "initializing" {
			record.InstallationState = "ready"
		}
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func olderRuntime(previous TargetState, snapshot remoteAgentState) bool {
	if snapshot.Tmux.SessionCreated < previous.SessionCreated {
		return true
	}
	if snapshot.Tmux.SessionCreated > previous.SessionCreated {
		return false
	}
	// tmux creation time is normally sufficient. If two runtimes report the
	// same second, keep the already accepted runtime rather than allowing an
	// ambiguous delayed read to roll the projection back.
	return snapshot.Tmux.SocketFingerprint != previous.SocketFingerprint
}

func (s *Service) updateSyncMetadata(endpointKey string, target Target, snapshot remoteAgentState) error {
	return s.store.Update(endpointKey, func(record *EndpointRecord) error {
		value := record.Targets[target.SessionName]
		value.SyncStatus = syncStatusAvailable
		value.LastSyncedAt = s.now().UTC()
		value.SyncError = ""
		record.Targets[target.SessionName] = value
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func (s *Service) appendTransitionMessage(view ports.ConnectionInstanceView, endpointKey string, target Target, record EndpointRecord, from string, snapshot remoteAgentState) error {
	if s.messages == nil {
		return nil
	}
	matching := s.matchingConnectionInstances(endpointKey, target.SessionName, record)
	if len(matching) == 0 && view.ConnectionInstanceID != "" {
		matching = []string{view.ConnectionInstanceID}
	}
	severity := "info"
	if snapshot.State == "error" {
		severity = "error"
	} else if snapshot.State == "relax" {
		severity = "success"
	}
	last := snapshot.Records[len(snapshot.Records)-1]
	definitionIDs := s.matchingConnectionDefinitions(matching, view)
	draft := domain.MessageDraft{
		Kind: "agent_state_transition", Severity: severity, AgentType: snapshot.Provider,
		PresentationKey: "agent_state_transition", OccurredAt: last.Timestamp.UTC(), ReceivedAt: s.now().UTC(),
		EndpointKey: endpointKey, FallbackLabel: messageFallbackLabel(record, target.SessionName),
		ConnectionLabel: s.notificationConnectionLabel(matching, record), TmuxSessionName: target.SessionName,
		TmuxSessionID: snapshot.Tmux.SessionID, TmuxSessionCreated: snapshot.Tmux.SessionCreated,
		ConnectionInstanceIDs: matching, AgentStateFrom: from, AgentStateTo: snapshot.State,
		ConnectionDefinitionIDs: definitionIDs,
		AgentRuntimeID:          snapshot.RuntimeID, AgentStateIndex: snapshot.LatestIndex,
		IdempotencyKey: snapshot.RuntimeID + "\x00" + fmt.Sprint(snapshot.LatestIndex),
	}
	if _, _, err := s.messages.AppendMessage(draft); err != nil {
		return fmt.Errorf("append Agent state transition: %w", err)
	}
	return nil
}

func (s *Service) matchingConnectionDefinitions(instanceIDs []string, fallback ports.ConnectionInstanceView) []string {
	seen := make(map[string]struct{}, len(instanceIDs)+1)
	result := make([]string, 0, len(instanceIDs)+1)
	for _, candidate := range s.terms.ConnectionInstanceViews() {
		matched := false
		for _, id := range instanceIDs {
			if id == candidate.ID || id == candidate.ConnectionInstanceID {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		definitionID := candidate.ConnectionDefinitionID
		if definitionID == "" {
			continue
		}
		if _, exists := seen[definitionID]; exists {
			continue
		}
		seen[definitionID] = struct{}{}
		result = append(result, definitionID)
	}
	if fallback.ConnectionDefinitionID != "" {
		if _, exists := seen[fallback.ConnectionDefinitionID]; !exists {
			result = append(result, fallback.ConnectionDefinitionID)
		}
	}
	return result
}

func legacyActivity(state string) string {
	switch state {
	case "running":
		return "running"
	case "relax":
		return "idle"
	case "error":
		return "failed"
	default:
		return "unknown"
	}
}

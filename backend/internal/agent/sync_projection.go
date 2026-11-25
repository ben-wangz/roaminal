package agent

import (
	"errors"
	"fmt"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var errStaleSnapshot = errors.New("agent state snapshot is older than the cached projection")

func (s *Service) acceptSnapshot(target Target, view ports.ConnectionInstanceView, views []ports.ConnectionInstanceView, snapshot remoteAgentState) error {
	lock := s.endpointMutex(target.EndpointKey)
	lock.Lock()
	defer lock.Unlock()

	var stale bool
	err := s.commitProjection(target.EndpointKey, func(record *EndpointRecord) (*domain.MessageDraft, error) {
		var draft *domain.MessageDraft
		draft, stale = s.projectSnapshot(record, target, view, views, snapshot)
		return draft, nil
	})
	if err != nil {
		return err
	}
	if stale {
		return errStaleSnapshot
	}
	return nil
}

func (s *Service) commitProjection(key string, update ports.AgentProjectionUpdate) error {
	if recorder, ok := s.messages.(ports.AgentProjectionRecorder); ok {
		return recorder.CommitAgentProjection(s.store, key, update)
	}
	if _, ok := s.store.(ports.AgentProjectionRepository); ok && s.messages != nil {
		return errors.New("Agent message recorder does not support atomic projection commits")
	}
	// Simple test appenders only work with non-transactional endpoint stores.
	return s.store.Update(key, func(record *EndpointRecord) error {
		draft, err := update(record)
		if err != nil || draft == nil {
			return err
		}
		if _, _, err := s.messages.AppendMessage(*draft); err != nil {
			return fmt.Errorf("append Agent state transition: %w", err)
		}
		return nil
	})
}

// projectSnapshot compares against the checkpoint under the repository's
// write lock, not the projection that was visible before reading remote state.
func (s *Service) projectSnapshot(record *EndpointRecord, target Target, view ports.ConnectionInstanceView, views []ports.ConnectionInstanceView, snapshot remoteAgentState) (*domain.MessageDraft, bool) {
	previous := record.Targets[target.SessionName]
	sameRuntime := previous.RuntimeID != "" && previous.RuntimeID == snapshot.RuntimeID
	if staleSnapshot(previous, snapshot, sameRuntime) {
		s.setSyncMetadata(record, target.SessionName, syncStatusStale, errStaleSnapshot)
		return nil, true
	}
	s.setSyncMetadata(record, target.SessionName, syncStatusAvailable, nil)
	if sameRuntime && snapshot.LatestIndex == previous.StateIndex && previous.State != "" {
		return nil, false
	}

	var draft *domain.MessageDraft
	value := record.Targets[target.SessionName]
	if sameRuntime && previous.State != "" && previous.State != snapshot.State {
		key := snapshot.RuntimeID + "\x00" + fmt.Sprint(snapshot.LatestIndex)
		if s.messages != nil && previous.LastTransitionKey != key {
			draft = s.transitionDraft(view, views, target, *record, previous.State, snapshot, key)
		}
		value.LastTransitionKey = key
	}
	value.Provider = snapshot.Provider
	value.SessionID = snapshot.Tmux.SessionID
	value.SessionCreated = snapshot.Tmux.SessionCreated
	value.SocketFingerprint = snapshot.Tmux.SocketFingerprint
	value.RuntimeID = snapshot.RuntimeID
	value.State = snapshot.State
	value.StateIndex = snapshot.LatestIndex
	value.StateUpdatedAt = snapshot.UpdatedAt.UTC()
	value.Component = "ready"
	value.ComponentVersion = snapshot.ComponentVersion
	value.Activity = activityForState(snapshot.State)
	value.LastEventAt = snapshot.Records[len(snapshot.Records)-1].Timestamp.UTC()
	value.LastEventName = snapshot.Records[len(snapshot.Records)-1].EventName
	value.ErrorCode, value.ErrorMessage = "", ""
	record.Targets[target.SessionName] = value
	record.ComponentVersion = snapshot.ComponentVersion
	if record.InstallationState == "" || record.InstallationState == "initializing" {
		record.InstallationState = "ready"
	}
	return draft, false
}

func staleSnapshot(previous TargetState, snapshot remoteAgentState, sameRuntime bool) bool {
	if sameRuntime {
		return snapshot.LatestIndex < previous.StateIndex ||
			(snapshot.LatestIndex == previous.StateIndex && previous.State != "" && previous.State != snapshot.State)
	}
	return previous.RuntimeID != "" && olderRuntime(previous, snapshot)
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

func activityForState(state string) string {
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

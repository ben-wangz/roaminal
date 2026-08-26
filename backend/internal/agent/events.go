package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

type webhookEvent struct {
	SchemaVersion    int           `json:"schemaVersion"`
	AgentType        string        `json:"agentType"`
	ComponentVersion string        `json:"componentVersion"`
	EventID          string        `json:"eventId"`
	EventName        string        `json:"eventName"`
	Activity         string        `json:"activity"`
	Sequence         uint64        `json:"sequence"`
	OccurredAt       time.Time     `json:"occurredAt"`
	Tmux             webhookTmux   `json:"tmux"`
	Codex            webhookCodex  `json:"codex"`
	Event            webhookSource `json:"event"`
}
type webhookTmux struct {
	SessionName       string `json:"sessionName"`
	SessionID         string `json:"sessionId"`
	SessionCreated    int64  `json:"sessionCreated"`
	PaneID            string `json:"paneId"`
	SocketFingerprint string `json:"socketFingerprint"`
}
type webhookCodex struct {
	SessionID      string `json:"sessionId"`
	TurnID         string `json:"turnId"`
	ToolUseID      string `json:"toolUseId"`
	AgentProcessID string `json:"agentProcessId"`
}
type webhookSource struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

func (s *Service) AcceptEvent(token string, body []byte) (bool, error) {
	if len(body) > 64*1024 {
		return false, errf("agent_event_too_large", 413, "The Agent event is too large.", nil)
	}
	endpointKey, record, ok := s.store.FindToken(token, s.now())
	if !ok {
		return false, errf("agent_event_unauthorized", 401, "The Agent event token is not valid.", nil)
	}
	if !s.allowEvent(endpointKey, s.now()) {
		return false, errf("agent_event_rate_limited", 429, "Agent event rate limit exceeded.", nil)
	}
	var event webhookEvent
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil || event.SchemaVersion != 1 || event.AgentType != "codex" ||
		event.ComponentVersion == "" || event.Sequence == 0 || event.Sequence > 1<<63-1 || event.EventName == "" ||
		event.Tmux.SessionName == "" || event.Tmux.SessionID == "" || event.Tmux.SessionCreated < 0 ||
		event.Codex.SessionID == "" || !knownEvent(event.EventName) {
		return false, errf("agent_event_invalid", 400, "The Agent event is invalid.", nil)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return false, errf("agent_event_invalid", 400, "The Agent event is invalid.", nil)
	}
	metadataValid := validOpaque(event.EventID, 128) && validOpaque(event.ComponentVersion, 32) &&
		validOpaque(event.Tmux.SessionName, 128) && validOpaque(event.Tmux.SessionID, 128) &&
		validOpaque(event.Tmux.PaneID, 128) && validOpaque(event.Codex.SessionID, 128) &&
		validOpaque(event.Codex.TurnID, 128) && validOpaque(event.Codex.ToolUseID, 128) &&
		validOpaque(event.Codex.AgentProcessID, 128) && validOpaque(event.Event.Source, 128) &&
		validOpaque(event.Event.Reason, 128)
	if !metadataValid {
		return false, errf("agent_event_invalid", 400, "The Agent event is invalid.", nil)
	}
	if !validSocketFingerprint(event.Tmux.SocketFingerprint) {
		return false, errf("agent_event_invalid", 400, "The Agent event is invalid.", nil)
	}
	if event.Tmux.PaneID == "" {
		return false, errf("agent_event_invalid", 400, "The Agent event is invalid.", nil)
	}
	if (event.EventName != "SessionStart" && event.Event.Source != "") ||
		(event.EventName == "SessionStart" && event.Event.Reason != "") ||
		(event.EventName != "SessionEnd" && event.Event.Reason != "") {
		return false, errf("agent_event_invalid", 400, "The Agent event contains an unrelated field.", nil)
	}
	if !validEventMetadata(event) {
		return false, errf("agent_event_invalid", 400, "The Agent event fields are invalid.", nil)
	}
	if expectedActivity(event.EventName, event.Event.Source) != event.Activity {
		return false, errf("agent_event_invalid", 400, "The Agent event activity is invalid.", nil)
	}
	if eventID(event, endpointKey) != event.EventID {
		return false, errf("agent_event_invalid", 400, "The Agent event ID is invalid.", nil)
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now().UTC()
	}
	now := s.now().UTC()
	runtimeKey := endpointKey + "\x00" + event.Tmux.SessionID + "\x00" + strconv.FormatInt(event.Tmux.SessionCreated, 10)
	runtimeLock := s.eventLockFor(runtimeKey)
	runtimeLock.Lock()
	defer runtimeLock.Unlock()
	s.mu.Lock()
	if s.runtime == nil {
		s.runtime = map[string]TargetState{}
	}
	if s.completedTools == nil {
		s.completedTools = map[string]map[string]time.Time{}
	}
	if s.runtimeTargets == nil {
		s.runtimeTargets = map[string]string{}
	}
	if s.eventIDs == nil {
		s.eventIDs = map[string]map[string]time.Time{}
	}
	if s.eventRememberedLocked(endpointKey, event.EventID, now) {
		s.mu.Unlock()
		return true, nil
	}
	targetName := s.runtimeTargets[runtimeKey]
	if targetName == "" {
		targetName = event.Tmux.SessionName
		for name, target := range record.Targets {
			if target.SessionID == event.Tmux.SessionID && target.SessionCreated == event.Tmux.SessionCreated {
				targetName = name
				break
			}
		}
	}
	key := endpointKey + "\x00" + targetName
	state := s.runtime[key]
	if state.SessionName == "" {
		state = record.Targets[targetName]
		state.SessionName = targetName
	}
	newRuntime := false
	if state.SessionID != "" && (state.SessionID != event.Tmux.SessionID || state.SessionCreated != event.Tmux.SessionCreated) {
		if event.Tmux.SessionCreated < state.SessionCreated {
			s.mu.Unlock()
			return true, nil
		}
		state = TargetState{SessionName: event.Tmux.SessionName, Component: "ready", ComponentVersion: event.ComponentVersion, InitializationID: state.InitializationID}
		newRuntime = true
	}
	completedTools := s.completedTools[key]
	completedTools = cloneCompletedTools(completedTools)
	if newRuntime {
		completedTools = map[string]time.Time{}
	}
	lateAfterStop := state.LastEventName == "Stop" && event.EventName != "SessionStart" && event.EventName != "SessionEnd" &&
		(event.EventName != "UserPromptSubmit" || event.Codex.TurnID == "" || event.Codex.TurnID == state.LastTurnID)
	compactAfterStop := event.EventName == "SessionStart" && event.Event.Source == "compact" && state.LastEventName == "Stop"
	lateToolPermission := event.EventName == "PermissionRequest" && event.Codex.ToolUseID != "" && completedTools[event.Codex.ToolUseID].After(now.Add(-24*time.Hour))
	if event.Sequence <= state.LastSequence ||
		(state.LastEventName == "SessionEnd" && event.EventName != "SessionStart") ||
		lateAfterStop || compactAfterStop ||
		(state.StoppedTurnID != "" && event.Codex.TurnID == state.StoppedTurnID && event.EventName != "SessionStart" && event.EventName != "SessionEnd") ||
		lateToolPermission {
		s.mu.Unlock()
		return true, nil
	}
	if event.EventName == "SessionStart" {
		state.LastEventName, state.LastTurnID, state.StoppedTurnID, state.LastToolUseID = "", "", "", ""
		state.LastSequence = 0
		completedTools = map[string]time.Time{}
	}
	state.SessionID, state.SessionCreated = event.Tmux.SessionID, event.Tmux.SessionCreated
	state.Component, state.ComponentVersion, state.Activity = "ready", event.ComponentVersion, event.Activity
	state.LastEventName, state.LastTurnID, state.LastToolUseID, state.LastSequence = event.EventName, event.Codex.TurnID, event.Codex.ToolUseID, event.Sequence
	if event.EventName == "Stop" && event.Codex.TurnID != "" {
		state.StoppedTurnID = event.Codex.TurnID
	}
	state.LastReceivedAt = s.now().UTC()
	state.LastEventAt = event.OccurredAt
	if s.since(event.OccurredAt) > 24*time.Hour || s.since(event.OccurredAt) < -24*time.Hour {
		state.LastEventAt = state.LastReceivedAt
	}
	if event.EventName == "PostToolUse" && event.Codex.ToolUseID != "" {
		completedTools[event.Codex.ToolUseID] = state.LastReceivedAt
		if len(completedTools) > 128 {
			var oldestID string
			var oldest time.Time
			for toolID, when := range completedTools {
				if oldestID == "" || when.Before(oldest) {
					oldestID, oldest = toolID, when
				}
			}
			delete(completedTools, oldestID)
		}
	}
	s.mu.Unlock()
	matchingIDs := s.matchingConnectionInstances(endpointKey, targetName, event.Tmux.SessionName, record)
	fallbackLabel := messageFallbackLabel(record, targetName)
	if s.messages != nil {
		if err := s.appendEventMessages(event, endpointKey, targetName, fallbackLabel, matchingIDs, now); err != nil {
			return false, err
		}
	}
	needsPersistentReady := record.InstallationState != "ready" || record.Targets[targetName].Component != "ready"
	if needsPersistentReady {
		if err := s.markComponentReady(endpointKey, targetName, event.Tmux.SessionID, event.Tmux.SessionCreated, event.ComponentVersion); err != nil {
			return false, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err)
		}
	}
	s.mu.Lock()
	if s.rememberEventLocked(endpointKey, event.EventID, now) {
		s.mu.Unlock()
		return true, nil
	}
	s.runtimeTargets[runtimeKey] = targetName
	s.completedTools[key] = completedTools
	s.runtime[key] = state
	s.mu.Unlock()
	return false, nil
}

func cloneCompletedTools(value map[string]time.Time) map[string]time.Time {
	result := make(map[string]time.Time, len(value))
	for id, receivedAt := range value {
		result[id] = receivedAt
	}
	return result
}

func (s *Service) appendEventMessages(event webhookEvent, endpointKey, targetName, fallbackLabel string, matchingIDs []string, receivedAt time.Time) error {
	draft := domain.MessageDraft{
		Kind: "agent_reporting_ready", Severity: "info", AgentType: event.AgentType, PresentationKey: "codex_reporting_connected",
		OccurredAt: event.OccurredAt.UTC(), ReceivedAt: receivedAt, EndpointKey: endpointKey, FallbackLabel: fallbackLabel,
		TmuxSessionName: targetName, TmuxSessionID: event.Tmux.SessionID, TmuxSessionCreated: event.Tmux.SessionCreated,
		ConnectionInstanceIDs: matchingIDs,
		IdempotencyKey:        strings.Join([]string{"agent_reporting_ready", endpointKey, event.Tmux.SessionID, strconv.FormatInt(event.Tmux.SessionCreated, 10), event.ComponentVersion}, "\x00"),
	}
	if _, _, err := s.messages.AppendMessage(draft); err != nil {
		return errf("message_store_unavailable", 503, "Agent message storage is unavailable.", err)
	}
	if event.EventName != "Stop" {
		return nil
	}
	draft.Kind = "codex_turn_completed"
	draft.Severity = "success"
	draft.PresentationKey = "codex_turn_finished"
	draft.IdempotencyKey = event.EventID + "\x00codex_turn_completed"
	if _, _, err := s.messages.AppendMessage(draft); err != nil {
		return errf("message_store_unavailable", 503, "Agent message storage is unavailable.", err)
	}
	return nil
}

func messageFallbackLabel(record EndpointRecord, targetName string) string {
	if record.User == "" || record.Host == "" || record.Port < 1 {
		return "tmux:" + targetName
	}
	return endpointDisplay(record.User, record.Host, record.Port) + " / tmux:" + targetName
}

func (s *Service) matchingConnectionInstances(endpointKey, targetName, eventSessionName string, record EndpointRecord) []string {
	if s.terms == nil {
		return nil
	}
	views := s.terms.ConnectionInstanceViews()
	s.mu.Lock()
	bindings := make(map[string]Target, len(s.bindings))
	for id, target := range s.bindings {
		bindings[id] = target
	}
	s.mu.Unlock()
	aliases := make(map[string]struct{}, len(record.Aliases))
	for _, alias := range record.Aliases {
		aliases[alias] = struct{}{}
	}
	result := make([]string, 0, len(views))
	for _, view := range views {
		if view.Lifecycle != "live" || view.ID == "" {
			continue
		}
		if target, bound := bindings[view.ID]; bound {
			if target.EndpointKey == endpointKey && target.SessionName == targetName {
				result = append(result, view.ConnectionInstanceID)
			}
			continue
		}
		alias := ""
		if view.SourceHostAlias != nil {
			alias = *view.SourceHostAlias
		}
		_, aliasMatches := aliases[alias]
		nameMatches := view.TmuxSessionName == targetName || (eventSessionName != "" && view.TmuxSessionName == eventSessionName)
		if aliasMatches && nameMatches {
			result = append(result, view.ConnectionInstanceID)
		}
	}
	return uniqueStrings(result)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := values[:0]
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

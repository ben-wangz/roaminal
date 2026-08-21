package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
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
	endpointKey, record, ok := s.store.FindToken(token, time.Now())
	if !ok {
		return false, errf("agent_event_unauthorized", 401, "The Agent event token is not valid.", nil)
	}
	if !s.allowEvent(endpointKey, time.Now()) {
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
	if len(event.Tmux.SocketFingerprint) != 16 {
		return false, errf("agent_event_invalid", 400, "The Agent event is invalid.", nil)
	}
	if _, err := hex.DecodeString(event.Tmux.SocketFingerprint); err != nil {
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
		event.OccurredAt = time.Now().UTC()
	}
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
	now := time.Now().UTC()
	if s.rememberEventLocked(endpointKey, event.EventID, now) {
		s.mu.Unlock()
		return true, nil
	}
	runtimeKey := endpointKey + "\x00" + event.Tmux.SessionID + "\x00" + strconv.FormatInt(event.Tmux.SessionCreated, 10)
	targetName := s.runtimeTargets[runtimeKey]
	if targetName == "" {
		targetName = event.Tmux.SessionName
		for name, target := range record.Targets {
			if target.SessionID == event.Tmux.SessionID && target.SessionCreated == event.Tmux.SessionCreated {
				targetName = name
				break
			}
		}
		s.runtimeTargets[runtimeKey] = targetName
	}
	key := endpointKey + "\x00" + targetName
	state := s.runtime[key]
	if state.SessionName == "" {
		state = record.Targets[targetName]
		state.SessionName = targetName
	}
	if state.SessionID != "" && (state.SessionID != event.Tmux.SessionID || state.SessionCreated != event.Tmux.SessionCreated) {
		if event.Tmux.SessionCreated < state.SessionCreated {
			s.mu.Unlock()
			return true, nil
		}
		state = TargetState{SessionName: event.Tmux.SessionName, Component: "ready", ComponentVersion: event.ComponentVersion, InitializationID: state.InitializationID}
		s.completedTools[key] = map[string]time.Time{}
	}
	completedTools := s.completedTools[key]
	if completedTools == nil {
		completedTools = map[string]time.Time{}
		s.completedTools[key] = completedTools
	}
	lateAfterStop := state.LastEventName == "Stop" && event.EventName != "SessionStart" && event.EventName != "SessionEnd" &&
		(event.EventName != "UserPromptSubmit" || event.Codex.TurnID == "" || event.Codex.TurnID == state.LastTurnID)
	compactAfterStop := event.EventName == "SessionStart" && event.Event.Source == "compact" && state.LastEventName == "Stop"
	lateToolPermission := event.EventName == "PermissionRequest" && event.Codex.ToolUseID != "" && completedTools[event.Codex.ToolUseID].After(time.Now().Add(-24*time.Hour))
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
		s.completedTools[key] = completedTools
	}
	state.SessionID, state.SessionCreated = event.Tmux.SessionID, event.Tmux.SessionCreated
	state.Component, state.ComponentVersion, state.Activity = "ready", event.ComponentVersion, event.Activity
	state.LastEventName, state.LastTurnID, state.LastToolUseID, state.LastSequence = event.EventName, event.Codex.TurnID, event.Codex.ToolUseID, event.Sequence
	if event.EventName == "Stop" && event.Codex.TurnID != "" {
		state.StoppedTurnID = event.Codex.TurnID
	}
	state.LastReceivedAt = time.Now().UTC()
	state.LastEventAt = event.OccurredAt
	if time.Since(event.OccurredAt) > 24*time.Hour || time.Since(event.OccurredAt) < -24*time.Hour {
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
	needsPersistentReady := record.InstallationState != "ready" || record.Targets[targetName].Component != "ready"
	s.runtime[key] = state
	s.mu.Unlock()
	if needsPersistentReady {
		if err := s.markComponentReady(endpointKey, targetName, event.Tmux.SessionID, event.Tmux.SessionCreated, event.ComponentVersion); err != nil {
			return false, errf("agent_store_unavailable", 503, "Agent state storage is unavailable.", err)
		}
	}
	return false, nil
}

func (s *Service) rememberEventLocked(endpointKey, eventID string, now time.Time) bool {
	items := s.eventIDs[endpointKey]
	if items == nil {
		items = map[string]time.Time{}
		s.eventIDs[endpointKey] = items
	}
	for id, seenAt := range items {
		if now.Sub(seenAt) > 24*time.Hour {
			delete(items, id)
		}
	}
	if _, exists := items[eventID]; exists {
		return true
	}
	if len(items) >= 2048 {
		oldestID := ""
		var oldest time.Time
		for id, seenAt := range items {
			if oldestID == "" || seenAt.Before(oldest) {
				oldestID, oldest = id, seenAt
			}
		}
		if oldestID != "" {
			delete(items, oldestID)
		}
	}
	items[eventID] = now
	return false
}

func (s *Service) markComponentReady(endpointKey, sessionName, sessionID string, sessionCreated int64, version string) error {
	return s.store.Update(endpointKey, func(record *EndpointRecord) error {
		record.InstallationState = "ready"
		if record.ComponentVersion == "" {
			record.ComponentVersion = version
		}
		state := record.Targets[sessionName]
		state.SessionName = sessionName
		state.SessionID, state.SessionCreated = sessionID, sessionCreated
		state.Component = "ready"
		if state.ComponentVersion == "" {
			state.ComponentVersion = version
		}
		state.Activity = "unknown"
		state.ErrorCode, state.ErrorMessage, state.InitializationID = "", "", ""
		record.Targets[sessionName] = state
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func expectedActivity(event, source string) string {
	switch event {
	case "PermissionRequest":
		return "waiting"
	case "Stop":
		return "completed"
	case "SessionStart":
		if source == "compact" {
			return "running"
		}
		return "idle"
	case "SessionEnd":
		return "idle"
	default:
		return "running"
	}
}

func knownEvent(value string) bool {
	switch value {
	case "SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "PreCompact", "PostCompact", "Stop":
		return true
	default:
		return false
	}
}

func validEventMetadata(event webhookEvent) bool {
	switch event.EventName {
	case "SessionStart":
		return (event.Event.Source == "startup" || event.Event.Source == "resume" || event.Event.Source == "clear" || event.Event.Source == "compact") && event.Codex.TurnID == "" && event.Codex.ToolUseID == ""
	case "SessionEnd":
		return event.Event.Reason == "other" && event.Codex.TurnID == "" && event.Codex.ToolUseID == ""
	case "PreToolUse", "PermissionRequest", "PostToolUse":
		return true
	case "UserPromptSubmit", "PreCompact", "PostCompact", "Stop":
		return event.Codex.ToolUseID == ""
	default:
		return false
	}
}

func eventID(event webhookEvent, endpointKey string) string {
	hash := sha256.New()
	writeString := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	writeString("roaminal-agent-event-v1")
	writeString(endpointKey)
	writeString(event.Tmux.SessionID)
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], uint64(event.Tmux.SessionCreated))
	_, _ = hash.Write(number[:])
	binary.BigEndian.PutUint64(number[:], event.Sequence)
	_, _ = hash.Write(number[:])
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}

func validOpaque(value string, maxBytes int) bool {
	if value == "" {
		return true
	}
	if len(value) > maxBytes {
		return false
	}
	return !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) })
}

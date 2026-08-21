package report

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

func ReadInput(r io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(io.LimitReader(r, 256*1024+1))
	if err != nil || len(data) > 256*1024 {
		return nil, errors.New("hook input too large")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, key := range []string{"hook_event_name", "session_id", "turn_id", "tool_use_id", "source", "reason"} {
		if value, ok := raw[key]; ok {
			var parsed string
			if json.Unmarshal(value, &parsed) == nil {
				if len(parsed) > 128 || strings.ContainsFunc(parsed, unicode.IsControl) {
					return nil, errors.New("hook metadata is invalid")
				}
				result[key] = parsed
			}
		}
	}
	if result["hook_event_name"] != "SessionStart" {
		delete(result, "source")
	}
	if result["hook_event_name"] != "SessionEnd" {
		delete(result, "reason")
	}
	return result, nil
}

func KnownEvent(value string) bool {
	switch value {
	case "SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "PreCompact", "PostCompact", "Stop":
		return true
	default:
		return false
	}
}

func Activity(event, source string) string {
	switch event {
	case "PermissionRequest":
		return "waiting"
	case "Stop":
		return "completed"
	case "SessionEnd":
		return "idle"
	case "SessionStart":
		if source == "compact" {
			return "running"
		}
		return "idle"
	default:
		return "running"
	}
}

func EventID(event model.Event) string {
	hash := sha256.New()
	writePart := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	writePart("roaminal-agent-event-v1")
	writePart(event.EndpointKey)
	writePart(event.Tmux.SessionID)
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], uint64(event.Tmux.SessionCreated))
	_, _ = hash.Write(number[:])
	binary.BigEndian.PutUint64(number[:], event.Sequence)
	_, _ = hash.Write(number[:])
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}

func NewEvent(input map[string]string, info tmux.Info, endpointKey, version string, sequence uint64) model.Event {
	event := model.Event{
		EndpointKey: endpointKey, SchemaVersion: model.SchemaVersion, AgentType: "codex",
		ComponentVersion: version, EventName: input["hook_event_name"],
		Activity: Activity(input["hook_event_name"], input["source"]), Sequence: sequence,
		OccurredAt: time.Now().UTC(),
		Tmux:       model.Tmux{SessionName: info.SessionName, SessionID: info.SessionID, SessionCreated: info.SessionCreated, PaneID: info.PaneID, SocketFingerprint: info.SocketFingerprint},
		Codex: model.Codex{
			SessionID:      input["session_id"],
			TurnID:         input["turn_id"],
			ToolUseID:      input["tool_use_id"],
			AgentProcessID: tmux.AgentProcessID(input["session_id"]),
		},
		Event: model.EventSource{Source: input["source"], Reason: input["reason"]},
	}
	event.EventID = EventID(event)
	return event
}

package report

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
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

// StateFor is the Codex provider adapter. Codex 0.147.0 has reliable activity
// and completion hooks but no reliable task-failure hook, so terminal events
// intentionally settle on relax unless a future provider event carries an
// explicit failure signal.
func StateFor(event, source, reason string) (string, bool) {
	if !KnownEvent(event) {
		return "", false
	}
	switch event {
	case "SessionStart":
		_ = source
		return model.StateRelax, true
	case "Stop", "SessionEnd":
		_ = reason
		return model.StateRelax, true
	default:
		return model.StateRunning, true
	}
}

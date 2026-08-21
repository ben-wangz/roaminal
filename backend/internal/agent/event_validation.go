package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode"
)

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

func validSocketFingerprint(value string) bool {
	if len(value) != 16 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

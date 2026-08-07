package terminal

import (
	"encoding/base64"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/internal/persistence"
)

func (s *Session) parseMarkersLocked(text string) string {
	text = s.markerPending + text
	s.markerPending = ""
	const titlePrefix = "\x1b]0;"
	const markerPrefix = "\x1b]777;roaminal;"
	var cleaned strings.Builder
	for index := 0; index < len(text); {
		relative := strings.IndexByte(text[index:], '\x1b')
		if relative < 0 {
			cleaned.WriteString(text[index:])
			break
		}
		escape := index + relative
		cleaned.WriteString(text[index:escape])
		remainder := text[escape:]
		if strings.HasPrefix(remainder, titlePrefix) {
			endRel := strings.IndexByte(remainder[len(titlePrefix):], '\x07')
			if endRel < 0 {
				s.markerPending = remainder
				break
			}
			title := truncateUTF8(remainder[len(titlePrefix):len(titlePrefix)+endRel], 512)
			s.meta.AutomaticTitle = title
			s.meta.SyncEffectiveTitle()
			s.meta.UpdatedAt = time.Now().UTC()
			_ = s.manager.store.SaveSession(s.meta)
			s.broadcastMetaLocked()
			cleaned.WriteString(remainder[:len(titlePrefix)+endRel+1])
			index = escape + len(titlePrefix) + endRel + 1
			continue
		}
		if strings.HasPrefix(remainder, markerPrefix) {
			endRel := strings.IndexByte(remainder[len(markerPrefix):], '\x07')
			if endRel < 0 {
				s.markerPending = remainder
				break
			}
			marker := remainder[len(markerPrefix) : len(markerPrefix)+endRel]
			s.applyMarkerLocked(marker)
			index = escape + len(markerPrefix) + endRel + 1
			continue
		}
		if isControlPrefix(remainder, titlePrefix) || isControlPrefix(remainder, markerPrefix) {
			s.markerPending = remainder
			break
		}
		cleaned.WriteByte(remainder[0])
		index = escape + 1
	}
	return cleaned.String()
}
func isControlPrefix(value, full string) bool {
	return len(value) < len(full) && strings.HasPrefix(full, value)
}
func truncateUTF8(value string, maxBytes int) string {
	data := []byte(value)
	if len(data) <= maxBytes {
		return value
	}
	data = data[:maxBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}
func decodeMarker(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func (s *Session) applyMarkerLocked(marker string) {
	kind, value, _ := strings.Cut(marker, ":")
	switch kind {
	case "cwd":
		if decoded, err := decodeMarker(value); err == nil && len(decoded) <= 4096 {
			if path := string(decoded); filepath.IsAbs(path) {
				s.meta.Cwd = path
				s.meta.UpdatedAt = time.Now().UTC()
				_ = s.manager.store.SaveSession(s.meta)
				s.broadcastMetaLocked()
			}
		}
	case "start":
		if decoded, err := decodeMarker(value); err == nil {
			command := string(decoded)
			if strings.Contains(command, "_roaminal_") || strings.Contains(command, "ROAMINAL_") {
				return
			}
			id, _ := newID()
			now := time.Now().UTC()
			s.currentExecID = id
			s.currentExec = &persistence.ExecutionRecord{Command: command, StartedAt: now}
			s.broadcastLocked(message(map[string]any{"type": "execution", "phase": "started", "executionId": id, "command": command, "startedAt": now}))
		}
	case "finish":
		if s.currentExec == nil {
			return
		}
		code, err := strconv.Atoi(value)
		if err != nil {
			code = 0
		}
		s.currentExec.ExitCode = &code
		s.currentExec.CompletedAt = time.Now().UTC()
		s.currentExec.DurationMs = s.currentExec.CompletedAt.Sub(s.currentExec.StartedAt).Milliseconds()
		record := *s.currentExec
		s.meta.Executions = append(s.meta.Executions, record)
		if len(s.meta.Executions) > 100 {
			s.meta.Executions = s.meta.Executions[len(s.meta.Executions)-100:]
		}
		s.meta.UpdatedAt = time.Now().UTC()
		if s.controlOwner == nil {
			s.attention = true
		}
		_ = s.manager.store.SaveSession(s.meta)
		s.broadcastLocked(message(map[string]any{"type": "execution", "phase": "completed", "executionId": s.currentExecID, "entry": record}))
		s.currentExec, s.currentExecID = nil, ""
	}
}

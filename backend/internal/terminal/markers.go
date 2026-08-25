package terminal

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// parseMarkersLocked strips Roaminal's private OSC markers from the raw PTY
// byte stream and applies their side effects. It operates on bytes before any
// UTF-8 decoding so a rune split across a marker boundary can be reassembled
// by the caller once the invisible marker bytes are removed.
func (s *Session) parseMarkersLocked(chunk []byte) []byte {
	text := append(s.markerPending, chunk...)
	s.markerPending = nil
	const titlePrefix = "\x1b]0;"
	const markerPrefix = "\x1b]777;roaminal;"
	var cleaned bytes.Buffer
	for index := 0; index < len(text); {
		relative := bytes.IndexByte(text[index:], '\x1b')
		if relative < 0 {
			cleaned.Write(text[index:])
			break
		}
		escape := index + relative
		cleaned.Write(text[index:escape])
		remainder := text[escape:]
		if bytes.HasPrefix(remainder, []byte(titlePrefix)) {
			endRel := bytes.IndexByte(remainder[len(titlePrefix):], '\x07')
			if endRel < 0 {
				s.markerPending = append([]byte(nil), remainder...)
				break
			}
			title := truncateUTF8(string(remainder[len(titlePrefix):len(titlePrefix)+endRel]), 512)
			s.meta.AutomaticTitle = title
			s.meta.SyncEffectiveTitle()
			s.meta.UpdatedAt = s.manager.now().UTC()
			if s.manager.hasPersistence() && !s.ephemeral {
				_ = s.manager.saveMeta(s.meta)
			}
			s.broadcastMetaLocked()
			cleaned.Write(remainder[:len(titlePrefix)+endRel+1])
			index = escape + len(titlePrefix) + endRel + 1
			continue
		}
		if bytes.HasPrefix(remainder, []byte(markerPrefix)) {
			endRel := bytes.IndexByte(remainder[len(markerPrefix):], '\x07')
			if endRel < 0 {
				s.markerPending = append([]byte(nil), remainder...)
				break
			}
			marker := string(remainder[len(markerPrefix) : len(markerPrefix)+endRel])
			s.applyMarkerLocked(marker)
			index = escape + len(markerPrefix) + endRel + 1
			continue
		}
		if isControlPrefix(remainder, titlePrefix) || isControlPrefix(remainder, markerPrefix) {
			s.markerPending = append([]byte(nil), remainder...)
			break
		}
		cleaned.WriteByte(remainder[0])
		index = escape + 1
	}
	return cleaned.Bytes()
}
func isControlPrefix(value []byte, full string) bool {
	return len(value) < len(full) && strings.HasPrefix(full, string(value))
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
				s.meta.UpdatedAt = s.manager.now().UTC()
				_ = s.manager.saveMeta(s.meta)
				s.broadcastMetaLocked()
			}
		}
	case "start":
		if decoded, err := decodeMarker(value); err == nil {
			command := string(decoded)
			if strings.Contains(command, "_roaminal_") || strings.Contains(command, "ROAMINAL_") {
				return
			}
			id, _ := s.manager.newID()
			now := s.manager.now().UTC()
			s.currentExecID = id
			s.currentExec = &executionRecord{Command: command, StartedAt: now}
			s.broadcastMessageLocked(executionStartedStreamMessage(id, command, now))
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
		s.currentExec.CompletedAt = s.manager.now().UTC()
		s.currentExec.DurationMs = s.currentExec.CompletedAt.Sub(s.currentExec.StartedAt).Milliseconds()
		record := *s.currentExec
		s.meta.UpdatedAt = s.manager.now().UTC()
		if s.controlOwner == nil {
			s.attention = true
		}
		if s.manager.hasPersistence() && !s.ephemeral {
			_ = s.manager.saveMeta(s.meta)
		}
		s.broadcastMessageLocked(executionCompletedStreamMessage(s.currentExecID, record))
		s.currentExec, s.currentExecID = nil, ""
	case "tmux-ready":
		if s.onMarker != nil {
			callback := s.onMarker
			s.onMarker = nil
			go callback(value)
		}
	}
}

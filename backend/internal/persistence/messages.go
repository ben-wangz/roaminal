package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const (
	maxStoredMessages = 500
	messageMaxAge     = 7 * 24 * time.Hour
)

type messageFile struct {
	FormatVersion       int                    `json:"formatVersion"`
	LatestSequence      uint64                 `json:"latestSequence"`
	Revision            uint64                 `json:"revision"`
	ReadThroughSequence uint64                 `json:"readThroughSequence"`
	Messages            []domain.MessageRecord `json:"messages"`
}

func (s *Store) messagesPath() string { return filepath.Join(s.Root, "messages.json") }

func emptyMessageFile() messageFile {
	return messageFile{FormatVersion: StorageSchemaVersion, Messages: []domain.MessageRecord{}}
}

func (s *Store) loadMessages() (messageFile, error) {
	data, err := os.ReadFile(s.messagesPath())
	if errors.Is(err, os.ErrNotExist) {
		return emptyMessageFile(), nil
	}
	if err != nil {
		return messageFile{}, fmt.Errorf("read message repository: %w", err)
	}
	var file messageFile
	if err := decodeStrict(data, &file); err != nil {
		return messageFile{}, fmt.Errorf("decode message repository: %w", err)
	}
	if err := validateMessageFile(file); err != nil {
		return messageFile{}, fmt.Errorf("validate message repository: %w", err)
	}
	return file, nil
}

func (s *Store) saveMessages(file messageFile) error {
	file.FormatVersion = StorageSchemaVersion
	if err := validateMessageFile(file); err != nil {
		return s.markError(err)
	}
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(s.messagesPath(), append(data, '\n')); err != nil {
		return s.markError(err)
	}
	return nil
}

func (s *Store) initializeMessages() error {
	s.messagesMu.Lock()
	defer s.messagesMu.Unlock()
	file, err := s.loadMessages()
	if err != nil {
		return s.markError(err)
	}
	if pruneMessageFile(&file, time.Now().UTC()) {
		file.Revision++
		if err := s.saveMessages(file); err != nil {
			return err
		}
	}
	return nil
}

func validateMessageFile(file messageFile) error {
	if file.FormatVersion != StorageSchemaVersion {
		return fmt.Errorf("unsupported message schema version %d", file.FormatVersion)
	}
	if file.ReadThroughSequence > file.LatestSequence {
		return errors.New("read cursor exceeds latest sequence")
	}
	seenIDs := make(map[string]struct{}, len(file.Messages))
	seenKeys := make(map[string]struct{}, len(file.Messages))
	var previous uint64
	for _, record := range file.Messages {
		if record.Sequence == 0 || record.Sequence <= previous || record.Sequence > file.LatestSequence {
			return errors.New("message sequence is not monotonic")
		}
		previous = record.Sequence
		if record.MessageID == "" {
			return errors.New("message id is empty")
		}
		if _, exists := seenIDs[record.MessageID]; exists {
			return errors.New("duplicate message id")
		}
		seenIDs[record.MessageID] = struct{}{}
		if record.IdempotencyKey == "" {
			return errors.New("message idempotency key is empty")
		}
		if _, exists := seenKeys[record.IdempotencyKey]; exists {
			return errors.New("duplicate message idempotency key")
		}
		seenKeys[record.IdempotencyKey] = struct{}{}
		if err := validateMessageDraft(record.MessageDraft); err != nil {
			return err
		}
	}
	if len(file.Messages) > maxStoredMessages {
		return errors.New("message retention limit exceeded")
	}
	if len(file.Messages) > 0 && file.Messages[len(file.Messages)-1].Sequence != previous {
		return errors.New("message sequence state is inconsistent")
	}
	return nil
}

func validateMessageDraft(draft domain.MessageDraft) error {
	if draft.MessageID == "" || draft.IdempotencyKey == "" || draft.EndpointKey == "" || draft.FallbackLabel == "" || draft.TmuxSessionName == "" || draft.TmuxSessionID == "" || draft.TmuxSessionCreated < 0 {
		return errors.New("message identity is incomplete")
	}
	if draft.ConnectionLabel != "" && !domain.IsSafeConnectionLabel(draft.ConnectionLabel) {
		return errors.New("message connection label is invalid")
	}
	if draft.AgentType != "codex" {
		return errors.New("unsupported message agent type")
	}
	switch draft.Kind {
	case "agent_reporting_ready":
		if draft.Severity != "info" || draft.PresentationKey != "codex_reporting_connected" {
			return errors.New("invalid reporting-ready presentation")
		}
	case "codex_turn_completed":
		if draft.Severity != "success" || draft.PresentationKey != "codex_turn_finished" {
			return errors.New("invalid turn-completed presentation")
		}
	case "codex_turn_failed":
		if draft.Severity != "error" || draft.PresentationKey != "codex_turn_failed" {
			return errors.New("invalid turn-failed presentation")
		}
	case "agent_state_transition":
		if draft.PresentationKey != "agent_state_transition" || !validAgentState(draft.AgentStateFrom) || !validAgentState(draft.AgentStateTo) || draft.AgentStateFrom == draft.AgentStateTo || draft.AgentRuntimeID == "" || draft.AgentStateIndex == 0 {
			return errors.New("invalid agent state transition presentation")
		}
	default:
		return errors.New("unsupported message kind")
	}
	if draft.OccurredAt.IsZero() || draft.ReceivedAt.IsZero() {
		return errors.New("message timestamp is empty")
	}
	seenInstances := make(map[string]struct{}, len(draft.ConnectionInstanceIDs))
	for _, id := range draft.ConnectionInstanceIDs {
		if id == "" {
			return errors.New("message connection instance id is empty")
		}
		if _, exists := seenInstances[id]; exists {
			return errors.New("duplicate message connection instance id")
		}
		seenInstances[id] = struct{}{}
	}
	return nil
}

func validAgentState(value string) bool {
	return value == "running" || value == "relax" || value == "error"
}

func pruneMessageFile(file *messageFile, now time.Time) bool {
	changed := false
	cutoff := now.Add(-messageMaxAge)
	retained := file.Messages[:0]
	for _, record := range file.Messages {
		if record.ReceivedAt.Before(cutoff) {
			changed = true
			continue
		}
		retained = append(retained, record)
	}
	if len(retained) > maxStoredMessages {
		retained = retained[len(retained)-maxStoredMessages:]
		changed = true
	}
	if changed {
		file.Messages = append([]domain.MessageRecord(nil), retained...)
		if file.ReadThroughSequence > file.LatestSequence {
			file.ReadThroughSequence = file.LatestSequence
		}
	}
	return changed
}

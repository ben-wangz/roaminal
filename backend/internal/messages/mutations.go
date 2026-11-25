package messages

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var (
	_ ports.MessageAppender         = (*Service)(nil)
	_ ports.AgentProjectionRecorder = (*Service)(nil)
)

func (s *Service) AppendMessage(draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	draft, err := s.prepareDraft(draft)
	if err != nil {
		return domain.MessageRecord{}, false, err
	}
	started := time.Now()
	record, duplicate, err := s.repository.AppendMessage(draft)
	if err != nil {
		log.Printf("level=INFO event=agent_message_store_failed code=%q kind=%q target_session=%q duration_ms=%d error_type=%T", ErrStoreUnavailable.Error(), draft.Kind, draft.TmuxSessionName, time.Since(started).Milliseconds(), err)
		return domain.MessageRecord{}, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	s.messageCommitted(record, duplicate, started)
	return record, duplicate, nil
}

func (s *Service) CommitAgentProjection(repo ports.AgentRepository, key string, update ports.AgentProjectionUpdate) error {
	started := time.Now()
	prepare := func(endpoint *domain.AgentEndpointRecord) (*domain.MessageDraft, error) {
		draft, err := update(endpoint)
		if err != nil || draft == nil {
			return nil, err
		}
		prepared, err := s.prepareDraft(*draft)
		return &prepared, err
	}
	var record domain.MessageRecord
	var duplicate bool
	var err error
	if atomic, ok := repo.(ports.AgentProjectionRepository); ok {
		record, duplicate, err = atomic.UpdateProjection(key, prepare)
	} else {
		// Separate legacy files cannot share a transaction. Still wait for
		// the checkpoint write to succeed before notifying.
		err = repo.Update(key, func(endpoint *domain.AgentEndpointRecord) error {
			draft, err := prepare(endpoint)
			if err != nil || draft == nil {
				return err
			}
			record, duplicate, err = s.repository.AppendMessage(*draft)
			return err
		})
	}
	if err != nil {
		return err
	}
	if record.MessageID != "" {
		s.messageCommitted(record, duplicate, started)
	}
	return nil
}

func (s *Service) prepareDraft(draft domain.MessageDraft) (domain.MessageDraft, error) {
	if s == nil || s.repository == nil || s.ids == nil {
		return domain.MessageDraft{}, ErrStoreUnavailable
	}
	if draft.MessageID == "" {
		id, err := s.ids.NewID()
		if err != nil || id == "" {
			if err == nil {
				err = errors.New("empty message id")
			}
			log.Printf("level=INFO event=agent_message_store_failed code=%q error_type=%T", ErrStoreUnavailable.Error(), err)
			return domain.MessageDraft{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
		}
		draft.MessageID = id
	}
	return draft, nil
}

func (s *Service) messageCommitted(record domain.MessageRecord, duplicate bool, started time.Time) {
	duration := time.Since(started).Milliseconds()
	if duplicate {
		log.Printf("level=INFO event=agent_message_duplicate message_id=%q kind=%q sequence=%d target_session=%q matching_connection_count=%d duration_ms=%d", record.MessageID, record.Kind, record.Sequence, record.TmuxSessionName, len(record.ConnectionInstanceIDs), duration)
		return
	}
	log.Printf("level=INFO event=agent_message_appended message_id=%q kind=%q sequence=%d target_session=%q matching_connection_count=%d duration_ms=%d", record.MessageID, record.Kind, record.Sequence, record.TmuxSessionName, len(record.ConnectionInstanceIDs), duration)
	if s.notifier != nil {
		s.notifier.Notify(record)
	}
}

func (s *Service) DeleteMessage(messageID string) (DeleteResult, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return DeleteResult{}, ErrMessageIDInvalid
	}
	if s == nil || s.repository == nil {
		return DeleteResult{}, ErrStoreUnavailable
	}
	started := time.Now()
	result, deleted, err := s.repository.DeleteMessage(messageID)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		log.Printf("level=INFO event=message_delete_failed code=%q duration_ms=%d error_type=%T", ErrStoreUnavailable.Error(), duration, err)
		return DeleteResult{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	log.Printf("level=INFO event=message_deleted deleted=%t revision=%d duration_ms=%d", deleted, result.Revision, duration)
	return DeleteResult{MessageID: messageID, Deleted: deleted, Revision: result.Revision, LatestSequence: result.LatestSequence, UnreadCount: result.UnreadCount}, nil
}

func (s *Service) ClearMessages() (ClearResult, error) {
	if s == nil || s.repository == nil {
		return ClearResult{}, ErrStoreUnavailable
	}
	started := time.Now()
	result, deletedCount, err := s.repository.ClearMessages()
	duration := time.Since(started).Milliseconds()
	if err != nil {
		log.Printf("level=INFO event=message_clear_failed code=%q duration_ms=%d error_type=%T", ErrStoreUnavailable.Error(), duration, err)
		return ClearResult{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	log.Printf("level=INFO event=messages_cleared deleted_count=%d revision=%d duration_ms=%d", deletedCount, result.Revision, duration)
	return ClearResult{DeletedCount: deletedCount, Revision: result.Revision, LatestSequence: result.LatestSequence, UnreadCount: result.UnreadCount}, nil
}

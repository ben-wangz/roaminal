package messages

import (
	"fmt"
	"log"
	"strings"
	"time"
)

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

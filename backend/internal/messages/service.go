package messages

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var (
	ErrStoreUnavailable = errors.New("message store unavailable")
	ErrCursorInvalid    = errors.New("message cursor invalid")
	ErrReadStateInvalid = errors.New("message read state invalid")
)

type Service struct {
	repository ports.MessageRepository
	ids        ports.IDGenerator
}

type Item struct {
	MessageID             string    `json:"messageId"`
	Sequence              uint64    `json:"sequence"`
	Kind                  string    `json:"kind"`
	Severity              string    `json:"severity"`
	Text                  string    `json:"text"`
	OccurredAt            time.Time `json:"occurredAt"`
	ReceivedAt            time.Time `json:"receivedAt"`
	ConnectionInstanceIDs []string  `json:"connectionInstanceIds"`
	FallbackLabel         string    `json:"fallbackLabel"`
	Read                  bool      `json:"read"`
}

type Page struct {
	Messages       []Item `json:"messages"`
	NextCursor     string `json:"nextCursor,omitempty"`
	Revision       uint64 `json:"revision"`
	LatestSequence uint64 `json:"latestSequence"`
	UnreadCount    int    `json:"unreadCount"`
}

type State struct {
	Revision       uint64 `json:"revision"`
	LatestSequence uint64 `json:"latestSequence"`
	UnreadCount    int    `json:"unreadCount"`
}

func New(repository ports.MessageRepository, ids ports.IDGenerator) *Service {
	return &Service{repository: repository, ids: ids}
}

func (s *Service) AppendMessage(draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	if s == nil || s.repository == nil || s.ids == nil {
		return domain.MessageRecord{}, false, ErrStoreUnavailable
	}
	if draft.MessageID == "" {
		id, err := s.ids.NewID()
		if err != nil || id == "" {
			if err == nil {
				err = errors.New("empty message id")
			}
			log.Printf("level=INFO event=agent_message_store_failed code=%q error_type=%T", ErrStoreUnavailable.Error(), err)
			return domain.MessageRecord{}, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
		}
		draft.MessageID = id
	}
	started := time.Now()
	record, duplicate, err := s.repository.AppendMessage(draft)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		log.Printf("level=INFO event=agent_message_store_failed code=%q kind=%q target_session=%q duration_ms=%d error_type=%T", ErrStoreUnavailable.Error(), draft.Kind, draft.TmuxSessionName, duration, err)
		return domain.MessageRecord{}, false, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	if duplicate {
		log.Printf("level=INFO event=agent_message_duplicate message_id=%q kind=%q sequence=%d target_session=%q matching_connection_count=%d duration_ms=%d", record.MessageID, record.Kind, record.Sequence, record.TmuxSessionName, len(record.ConnectionInstanceIDs), duration)
		return record, true, nil
	}
	log.Printf("level=INFO event=agent_message_appended message_id=%q kind=%q sequence=%d target_session=%q matching_connection_count=%d duration_ms=%d", record.MessageID, record.Kind, record.Sequence, record.TmuxSessionName, len(record.ConnectionInstanceIDs), duration)
	return record, false, nil
}

func (s *Service) List(limit int, cursor string) (Page, error) {
	if s == nil || s.repository == nil {
		return Page{}, ErrStoreUnavailable
	}
	if limit < 1 || limit > 100 {
		return Page{}, ErrCursorInvalid
	}
	before, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, ErrCursorInvalid
	}
	result, err := s.repository.ListMessages(limit, before)
	if err != nil {
		if errors.Is(err, ports.ErrMessageCursorInvalid) {
			return Page{}, ErrCursorInvalid
		}
		return Page{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	page := Page{Messages: make([]Item, 0, len(result.Messages)), Revision: result.Revision, LatestSequence: result.LatestSequence, UnreadCount: result.UnreadCount}
	for _, record := range result.Messages {
		page.Messages = append(page.Messages, item(record, result.ReadThroughSequence))
	}
	if result.HasMore {
		page.NextCursor = encodeCursor(result.NextBefore)
	}
	return page, nil
}

func (s *Service) MarkReadThrough(sequence uint64) (State, error) {
	if s == nil || s.repository == nil {
		return State{}, ErrStoreUnavailable
	}
	result, err := s.repository.MarkMessagesReadThrough(sequence)
	if err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return state(result), nil
}

func (s *Service) State() (State, error) {
	if s == nil || s.repository == nil {
		return State{}, ErrStoreUnavailable
	}
	result, err := s.repository.MessageState()
	if err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return state(result), nil
}

func item(record domain.MessageRecord, readThrough uint64) Item {
	return Item{
		MessageID: record.MessageID, Sequence: record.Sequence, Kind: record.Kind, Severity: record.Severity,
		Text: presentationText(record.PresentationKey), OccurredAt: record.OccurredAt, ReceivedAt: record.ReceivedAt,
		ConnectionInstanceIDs: append([]string{}, record.ConnectionInstanceIDs...), FallbackLabel: record.FallbackLabel,
		Read: record.Sequence <= readThrough,
	}
}

func state(value domain.MessageState) State {
	return State{Revision: value.Revision, LatestSequence: value.LatestSequence, UnreadCount: value.UnreadCount}
}

func presentationText(key string) string {
	switch key {
	case "codex_reporting_connected":
		return "Codex reporting connected"
	case "codex_turn_finished":
		return "Codex turn finished"
	case "codex_turn_failed":
		return "Codex turn failed"
	default:
		return "Agent message"
	}
}

func encodeCursor(sequence uint64) string {
	return base64.RawURLEncoding.EncodeToString([]byte("before:" + strconv.FormatUint(sequence, 10)))
}

func decodeCursor(cursor string) (uint64, error) {
	if strings.TrimSpace(cursor) == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(decoded), "before:") {
		return 0, ErrCursorInvalid
	}
	sequence, err := strconv.ParseUint(strings.TrimPrefix(string(decoded), "before:"), 10, 64)
	if err != nil || sequence == 0 {
		return 0, ErrCursorInvalid
	}
	return sequence, nil
}

var _ ports.MessageAppender = (*Service)(nil)

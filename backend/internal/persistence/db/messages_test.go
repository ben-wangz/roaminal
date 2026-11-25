package db

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
)

func messageDraft(index int, received time.Time) domain.MessageDraft {
	return domain.MessageDraft{
		MessageID: fmt.Sprintf("message-%d", index), IdempotencyKey: fmt.Sprintf("agent\x00%d", index),
		Kind: "agent_state_transition", Severity: "success", AgentType: "codex", PresentationKey: "agent_state_transition",
		OccurredAt: received.Add(-8 * 24 * time.Hour), ReceivedAt: received, EndpointKey: "endpoint", FallbackLabel: "tmux:team",
		TmuxSessionName: "team", TmuxSessionID: "$0", TmuxSessionCreated: 1,
		AgentStateFrom: "running", AgentStateTo: "relax", AgentRuntimeID: "agent", AgentStateIndex: uint64(index),
		ConnectionInstanceIDs: []string{"instance"}, ConnectionDefinitionIDs: []string{"definition"},
	}
}

func TestSQLiteMessageRepository(t *testing.T) {
	s, _ := sqliteStore(t)
	testMessageRepository(t, s)
}

func testMessageRepository(t *testing.T, s *Store) {
	t.Helper()
	if state, err := s.MessageState(); err != nil || state != (domain.MessageState{}) {
		t.Fatalf("fresh message state = %+v, %v", state, err)
	}
	now := time.Now().UTC().Truncate(time.Second).Add(123456789)
	for i := 1; i <= 4; i++ {
		draft := messageDraft(i, now)
		record, duplicate, err := s.AppendMessage(draft)
		if err != nil || duplicate || record.Sequence != uint64(i) {
			t.Fatalf("append %d = %+v, %t, %v", i, record, duplicate, err)
		}
		draft.ConnectionInstanceIDs[0] = "changed by caller"
		record.ConnectionDefinitionIDs[0] = "changed on return"
	}
	retry := messageDraft(1, now)
	retry.MessageID = "different-id"
	record, duplicate, err := s.AppendMessage(retry)
	if err != nil || !duplicate || record.Sequence != 1 || record.MessageID != "message-1" || !record.ReceivedAt.Equal(now) ||
		record.ConnectionInstanceIDs[0] != "instance" || record.ConnectionDefinitionIDs[0] != "definition" {
		t.Fatalf("idempotent append = %+v, %t, %v", record, duplicate, err)
	}
	retry.IdempotencyKey = "different-key"
	retry.MessageID = "message-1"
	if _, _, err := s.AppendMessage(retry); err == nil {
		t.Fatal("duplicate message ID succeeded")
	}
	retry.ConnectionLabel = "label\nwith line break"
	if _, _, err := s.AppendMessage(retry); err == nil {
		t.Fatal("invalid message draft succeeded")
	}
	if state, err := s.MarkMessagesReadThrough(1); err != nil || state.Revision != 5 || state.UnreadCount != 3 {
		t.Fatalf("read through 1 = %+v, %v", state, err)
	}
	if state, deleted, err := s.DeleteMessage("message-2"); err != nil || !deleted || state.Revision != 6 || state.UnreadCount != 2 {
		t.Fatalf("delete unread = %+v, %t, %v", state, deleted, err)
	}
	if state, deleted, err := s.DeleteMessage("message-2"); err != nil || deleted || state.Revision != 6 {
		t.Fatalf("delete missing = %+v, %t, %v", state, deleted, err)
	}
	page, err := s.ListMessages(2, 0)
	if err != nil || len(page.Messages) != 2 || page.Messages[0].Sequence != 4 || page.Messages[1].Sequence != 3 ||
		!page.HasMore || page.NextBefore != 3 || page.Revision != 6 || page.LatestSequence != 4 || page.ReadThroughSequence != 1 || page.UnreadCount != 2 {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	page, err = s.ListMessages(2, page.NextBefore)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Sequence != 1 || page.HasMore || page.NextBefore != 0 || page.UnreadCount != 2 {
		t.Fatalf("second page = %+v, %v", page, err)
	}
	if _, err := s.ListMessages(2, 5); !errors.Is(err, ports.ErrMessageCursorInvalid) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	if _, err := s.ListMessages(101, 0); err == nil {
		t.Fatal("invalid page limit succeeded")
	}
	state, err := s.MarkMessagesReadThrough(^uint64(0))
	if err != nil || state.ReadThroughSequence != 4 || state.UnreadCount != 0 || state.Revision != 7 {
		t.Fatalf("read cursor clamp = %+v, %v", state, err)
	}
	if got, err := s.MarkMessagesReadThrough(1); err != nil || got != state {
		t.Fatalf("read cursor rewound = %+v, %v", got, err)
	}
	state, count, err := s.ClearMessages()
	if err != nil || count != 3 || state.LatestSequence != 4 || state.ReadThroughSequence != 4 || state.Revision != 8 || state.UnreadCount != 0 {
		t.Fatalf("clear = %+v, %d, %v", state, count, err)
	}
	if got, count, err := s.ClearMessages(); err != nil || count != 0 || got != state {
		t.Fatalf("clear empty = %+v, %d, %v", got, count, err)
	}
	if record, duplicate, err := s.AppendMessage(messageDraft(2, now)); err != nil || duplicate || record.Sequence != 5 {
		t.Fatalf("append after clear = %+v, %t, %v", record, duplicate, err)
	}
}

func TestSQLiteMessageCountRetentionAndDelayedAppend(t *testing.T) {
	s, _ := sqliteStore(t)
	now := time.Now().UTC()
	old := messageDraft(1, now.Add(-8*24*time.Hour))
	if got, duplicate, err := s.AppendMessage(old); err != nil || duplicate || got.Sequence != 1 {
		t.Fatalf("delayed append = %+v, %t, %v", got, duplicate, err)
	}
	if got, deleted, err := s.ClearMessages(); err != nil || deleted != 0 || got.LatestSequence != 1 || got.Revision != 1 || got.ReadThroughSequence != 0 {
		t.Fatalf("clear with only delayed history = %+v, %d, %v", got, deleted, err)
	}
	for i := 1; i <= persistence.MaxStoredMessages; i++ {
		if _, duplicate, err := s.AppendMessage(messageDraft(i, now)); err != nil || duplicate {
			t.Fatalf("append %d: duplicate = %t, err = %v", i, duplicate, err)
		}
	}
	// Reusing the ID of the record evicted for capacity must succeed; the
	// uniqueness check only applies to the retained records.
	newest := messageDraft(persistence.MaxStoredMessages+1, now)
	newest.MessageID = old.MessageID
	if got, duplicate, err := s.AppendMessage(newest); err != nil || duplicate || got.Sequence != persistence.MaxStoredMessages+2 {
		t.Fatalf("append at capacity = %+v, %t, %v", got, duplicate, err)
	}
	page, err := s.ListMessages(100, 0)
	if err != nil || len(page.Messages) != 100 || !page.HasMore || page.NextBefore != persistence.MaxStoredMessages-97 ||
		page.Revision != persistence.MaxStoredMessages+2 || page.UnreadCount != persistence.MaxStoredMessages {
		t.Fatalf("bounded page = %+v, %v", page, err)
	}
}

func TestSQLiteMessageImportAndStartupPruning(t *testing.T) {
	for _, tc := range []struct {
		name          string
		beforeServing bool
	}{
		{name: "duplicate-append"},
		{name: "serving-startup", beforeServing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := sqliteStore(t)
			now := time.Now().UTC()
			old := domain.MessageRecord{Sequence: 2, MessageDraft: messageDraft(1, now.Add(-8*24*time.Hour))}
			fresh := domain.MessageRecord{Sequence: 4, MessageDraft: messageDraft(2, now)}
			seed := domain.MessageState{LatestSequence: 6, Revision: 10, ReadThroughSequence: 2}
			if err := s.DB.Transaction(func(tx *gorm.DB) error {
				return importMessages(tx, seed, []domain.MessageRecord{old, fresh})
			}); err != nil {
				t.Fatal(err)
			}
			page, err := s.ListMessages(10, 0)
			if err != nil || page.Revision != 10 || len(page.Messages) != 2 || page.UnreadCount != 1 {
				t.Fatalf("import pruned data eagerly = %+v, %v", page, err)
			}
			if tc.beforeServing {
				s = reopenMessageNotificationStore(t, c)
				if state, err := s.MessageState(); err != nil || state.Revision != 10 {
					t.Fatalf("offline reopen mutated state = %+v, %v", state, err)
				}
				if err := s.DB.Transaction(s.initializeMessageNotifications); err != nil {
					t.Fatal(err)
				}
			} else if record, duplicate, err := s.AppendMessage(fresh.MessageDraft); err != nil || !duplicate || record.Sequence != 4 {
				t.Fatalf("duplicate with age pruning = %+v, %t, %v", record, duplicate, err)
			}
			page, err = s.ListMessages(10, 0)
			if err != nil || page.Revision != 11 || page.LatestSequence != 6 || page.ReadThroughSequence != 2 ||
				len(page.Messages) != 1 || page.Messages[0].Sequence != 4 || page.UnreadCount != 1 {
				t.Fatalf("pruned snapshot = %+v, %v", page, err)
			}
		})
	}
}

func TestSQLiteMessageImportPreservesUnsignedCounters(t *testing.T) {
	s, _ := sqliteStore(t)
	now := time.Now().UTC()
	const high = 1 << 63
	state := domain.MessageState{LatestSequence: high + 1, Revision: high + 2, ReadThroughSequence: high - 1}
	records := []domain.MessageRecord{{Sequence: high - 1, MessageDraft: messageDraft(1, now)}, {Sequence: high, MessageDraft: messageDraft(2, now)}}
	if err := s.DB.Transaction(func(tx *gorm.DB) error { return importMessages(tx, state, records) }); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error { return importMessages(tx, state, records) }); err == nil {
		t.Fatal("overwrote existing message state")
	}
	page, err := s.ListMessages(1, 0)
	if err != nil || page.LatestSequence != high+1 || page.ReadThroughSequence != high-1 || page.Revision != high+2 ||
		page.UnreadCount != 1 || !page.HasMore || page.NextBefore != high || page.Messages[0].Sequence != high {
		t.Fatalf("unsigned page = %+v, %v", page, err)
	}
	page, err = s.ListMessages(1, high)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Sequence != high-1 {
		t.Fatalf("unsigned cursor = %+v, %v", page, err)
	}
	if record, duplicate, err := s.AppendMessage(messageDraft(3, now)); err != nil || duplicate || record.Sequence != high+2 {
		t.Fatalf("unsigned append = %+v, %t, %v", record, duplicate, err)
	}
}

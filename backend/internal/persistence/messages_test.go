package persistence

import (
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestMessageRepositoryIsIdempotentAndRetainsMonotonicState(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	now := time.Now().UTC()
	for index := 1; index <= 501; index++ {
		_, duplicate, err := repository.AppendMessage(testMessageDraft(index, now))
		if err != nil || duplicate {
			t.Fatalf("append %d: duplicate=%v err=%v", index, duplicate, err)
		}
	}
	record, duplicate, err := repository.AppendMessage(testMessageDraft(501, now))
	if err != nil || !duplicate || record.Sequence != 501 {
		t.Fatalf("duplicate append: record=%+v duplicate=%v err=%v", record, duplicate, err)
	}
	page, err := repository.ListMessages(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 100 || !page.HasMore || page.NextBefore != 402 || page.LatestSequence != 501 {
		t.Fatalf("unexpected page: %+v", page)
	}
	state, err := repository.MarkMessagesReadThrough(9999)
	if err != nil {
		t.Fatal(err)
	}
	if state.ReadThroughSequence != 501 || state.UnreadCount != 0 {
		t.Fatalf("unexpected read state: %+v", state)
	}
	state, err = repository.MarkMessagesReadThrough(1)
	if err != nil {
		t.Fatal(err)
	}
	if state.ReadThroughSequence != 501 || state.UnreadCount != 0 {
		t.Fatalf("read cursor moved backwards: %+v", state)
	}
	restarted, err := New(store.Root)
	if err != nil {
		t.Fatal(err)
	}
	state, err = NewRepositories(restarted).Messages.MessageState()
	if err != nil || state.LatestSequence != 501 || state.ReadThroughSequence != 501 || state.UnreadCount != 0 {
		t.Fatalf("unexpected restarted state: %+v err=%v", state, err)
	}
}

func TestMessageRepositoryPrunesAgeOnAppend(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	if _, _, err := repository.AppendMessage(testMessageDraft(1, old)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.AppendMessage(testMessageDraft(2, time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListMessages(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].MessageID != "message-002" || page.LatestSequence != 2 {
		t.Fatalf("old message was not pruned: %+v", page)
	}
}

func TestMessageRepositoryUsesReceiptTimeForDelayedMessagePruning(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	current := time.Now().UTC()
	if _, _, err := repository.AppendMessage(testMessageDraft(1, current)); err != nil {
		t.Fatal(err)
	}
	old := current.Add(-8 * 24 * time.Hour)
	if _, _, err := repository.AppendMessage(testMessageDraft(2, old)); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListMessages(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].MessageID != "message-001" || page.LatestSequence != 2 {
		t.Fatalf("delayed message was not pruned using receipt time: %+v", page)
	}
}

func TestMessageRepositoryPrunesBeforeDuplicateAppend(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	now := time.Now().UTC()
	old := testMessageDraft(1, now.Add(-8*24*time.Hour))
	recent := testMessageDraft(2, now)
	if err := store.saveMessages(messageFile{
		FormatVersion: StorageSchemaVersion, LatestSequence: 2, Revision: 2,
		Messages: []domain.MessageRecord{
			{MessageDraft: old, Sequence: 1},
			{MessageDraft: recent, Sequence: 2},
		},
	}); err != nil {
		t.Fatal(err)
	}
	record, duplicate, err := repository.AppendMessage(recent)
	if err != nil || !duplicate || record.Sequence != 2 {
		t.Fatalf("duplicate append: record=%+v duplicate=%v err=%v", record, duplicate, err)
	}
	page, err := repository.ListMessages(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].Sequence != 2 || page.Revision != 3 {
		t.Fatalf("duplicate append did not prune old messages: %+v", page)
	}
}

func testMessageDraft(index int, receivedAt time.Time) domain.MessageDraft {
	return domain.MessageDraft{
		MessageID: "message-" + formatTestIndex(index), IdempotencyKey: "key-" + formatTestIndex(index),
		Kind: "codex_turn_completed", Severity: "success", AgentType: "codex", PresentationKey: "codex_turn_finished",
		OccurredAt: receivedAt, ReceivedAt: receivedAt, EndpointKey: "endpoint", FallbackLabel: "tmux:roaminal",
		TmuxSessionName: "roaminal", TmuxSessionID: "$0", TmuxSessionCreated: 1,
	}
}

func formatTestIndex(index int) string {
	if index < 10 {
		return "00" + string(rune('0'+index))
	}
	if index < 100 {
		return "0" + string(rune('0'+index/10)) + string(rune('0'+index%10))
	}
	return string(rune('0'+index/100)) + string(rune('0'+(index/10)%10)) + string(rune('0'+index%10))
}

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

func TestMessageRepositoryDeletesMessagesWithoutRewindingState(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	now := time.Now().UTC()
	for index := 1; index <= 3; index++ {
		if _, _, err := repository.AppendMessage(testMessageDraft(index, now)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.MarkMessagesReadThrough(1); err != nil {
		t.Fatal(err)
	}
	state, deleted, err := repository.DeleteMessage("message-002")
	if err != nil || !deleted || state.LatestSequence != 3 || state.ReadThroughSequence != 1 || state.UnreadCount != 1 {
		t.Fatalf("delete unread message: state=%+v deleted=%v err=%v", state, deleted, err)
	}
	state, deleted, err = repository.DeleteMessage("message-002")
	if err != nil || deleted || state.Revision != 5 || state.UnreadCount != 1 {
		t.Fatalf("repeat delete: state=%+v deleted=%v err=%v", state, deleted, err)
	}
	state, deleted, err = repository.DeleteMessage("message-001")
	if err != nil || !deleted || state.ReadThroughSequence != 1 || state.UnreadCount != 1 {
		t.Fatalf("delete read message: state=%+v deleted=%v err=%v", state, deleted, err)
	}
	page, err := repository.ListMessages(100, 0)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].MessageID != "message-003" {
		t.Fatalf("remaining messages: page=%+v err=%v", page, err)
	}
}

func TestMessageRepositoryClearsMessagesAndContinuesSequence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	now := time.Now().UTC()
	for index := 1; index <= 2; index++ {
		if _, _, err := repository.AppendMessage(testMessageDraft(index, now)); err != nil {
			t.Fatal(err)
		}
	}
	state, deletedCount, err := repository.ClearMessages()
	if err != nil || deletedCount != 2 || state.LatestSequence != 2 || state.ReadThroughSequence != 2 || state.UnreadCount != 0 {
		t.Fatalf("clear messages: state=%+v deleted=%d err=%v", state, deletedCount, err)
	}
	clearedRevision := state.Revision
	state, deletedCount, err = repository.ClearMessages()
	if err != nil || deletedCount != 0 || state.Revision != clearedRevision {
		t.Fatalf("clear empty store: state=%+v deleted=%d err=%v", state, deletedCount, err)
	}
	record, duplicate, err := repository.AppendMessage(testMessageDraft(3, now))
	if err != nil || duplicate || record.Sequence != 3 {
		t.Fatalf("sequence after clear: record=%+v duplicate=%v err=%v", record, duplicate, err)
	}
}

func TestMessageRepositoryRejectsUnsafeConnectionLabel(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	draft := testMessageDraft(1, time.Now().UTC())
	draft.ConnectionLabel = "remote\nendpoint"
	if _, _, err := NewRepositories(store).Messages.AppendMessage(draft); err == nil {
		t.Fatal("unsafe connection label was accepted")
	}
	draft.ConnectionLabel = string(make([]byte, 129))
	if _, _, err := NewRepositories(store).Messages.AppendMessage(draft); err == nil {
		t.Fatal("oversized connection label was accepted")
	}
}

package persistence

import (
	"sync"
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

func TestMessageRepositorySerializesConcurrentMutations(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Messages
	now := time.Now().UTC()
	for index := 1; index <= 40; index++ {
		if _, _, err := repository.AppendMessage(testMessageDraft(index, now)); err != nil {
			t.Fatal(err)
		}
	}

	const appended = 40
	start := make(chan struct{})
	results := make(chan error, appended+2)
	var group sync.WaitGroup
	group.Add(appended + 2)
	for index := 41; index <= 80; index++ {
		go func(index int) {
			defer group.Done()
			<-start
			_, _, appendErr := repository.AppendMessage(testMessageDraft(index, now))
			results <- appendErr
		}(index)
	}
	go func() {
		defer group.Done()
		<-start
		_, _, deleteErr := repository.DeleteMessage("message-001")
		results <- deleteErr
	}()
	go func() {
		defer group.Done()
		<-start
		_, _, clearErr := repository.ClearMessages()
		results <- clearErr
	}()
	close(start)
	group.Wait()
	close(results)
	for mutationErr := range results {
		if mutationErr != nil {
			t.Fatalf("concurrent message mutation failed: %v", mutationErr)
		}
	}

	page, err := repository.ListMessages(100, 0)
	if err != nil {
		t.Fatal(err)
	}
	state, err := repository.MessageState()
	if err != nil {
		t.Fatal(err)
	}
	if page.LatestSequence != 80 || state.LatestSequence != 80 || state.ReadThroughSequence > state.LatestSequence {
		t.Fatalf("concurrent mutations broke monotonic state: page=%+v state=%+v", page, state)
	}
	if len(page.Messages) > 80 {
		t.Fatalf("concurrent mutations created duplicate records: %d", len(page.Messages))
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

package messages

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestServicePaginatesWithFixedPresentationAndIdempotency(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ids := &testIDs{}
	notifier := &testNotifier{}
	service := NewWithNotifier(persistence.NewRepositories(store).Messages, ids, notifier)
	now := time.Now().UTC()
	draft := domain.MessageDraft{
		Kind: "codex_turn_completed", Severity: "success", AgentType: "codex", PresentationKey: "codex_turn_finished",
		OccurredAt: now, ReceivedAt: now, EndpointKey: "private-key", FallbackLabel: "fallback",
		ConnectionLabel: "remote-alias",
		TmuxSessionName: "roaminal", TmuxSessionID: "$0", TmuxSessionCreated: 1, IdempotencyKey: "event-1",
	}
	first, duplicate, err := service.AppendMessage(draft)
	if err != nil || duplicate || first.MessageID != "message-1" {
		t.Fatalf("first append: record=%+v duplicate=%v err=%v", first, duplicate, err)
	}
	secondDraft := draft
	secondDraft.IdempotencyKey = "event-2"
	secondDraft.Kind = "agent_reporting_ready"
	secondDraft.Severity = "info"
	secondDraft.PresentationKey = "codex_reporting_connected"
	second, duplicate, err := service.AppendMessage(secondDraft)
	if err != nil || duplicate || second.Sequence != 2 {
		t.Fatalf("second append: record=%+v duplicate=%v err=%v", second, duplicate, err)
	}
	repeated, duplicate, err := service.AppendMessage(draft)
	if err != nil || !duplicate || repeated.MessageID != first.MessageID {
		t.Fatalf("repeated append: record=%+v duplicate=%v err=%v", repeated, duplicate, err)
	}
	if len(notifier.records) != 2 {
		t.Fatalf("notifier calls=%d, want 2 new records", len(notifier.records))
	}
	page, err := service.List(1, "")
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Text != "Codex reporting connected" || page.NextCursor == "" {
		t.Fatalf("first page: %+v err=%v", page, err)
	}
	if page.Messages[0].ConnectionInstanceIDs == nil {
		t.Fatal("message connection instance ids must encode as an empty array")
	}
	if page.Messages[0].ConnectionLabel != "remote-alias" {
		t.Fatalf("safe connection label was not returned: %+v", page.Messages[0])
	}
	older, err := service.List(1, page.NextCursor)
	if err != nil || len(older.Messages) != 1 || older.Messages[0].Text != "Codex turn finished" || older.NextCursor != "" {
		t.Fatalf("older page: %+v err=%v", older, err)
	}
	if _, err := service.List(1, "invalid"); !errors.Is(err, ErrCursorInvalid) {
		t.Fatalf("invalid cursor error=%v", err)
	}
}

func TestServiceReturnsSafeMessageMutationResults(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ids := &testIDs{}
	service := New(persistence.NewRepositories(store).Messages, ids)
	now := time.Now().UTC()
	for index := 1; index <= 2; index++ {
		if _, _, err := service.AppendMessage(domain.MessageDraft{
			Kind: "codex_turn_completed", Severity: "success", AgentType: "codex", PresentationKey: "codex_turn_finished",
			OccurredAt: now, ReceivedAt: now, EndpointKey: "private-key", FallbackLabel: "fallback",
			TmuxSessionName: "roaminal", TmuxSessionID: "$0", TmuxSessionCreated: 1, IdempotencyKey: "event-" + string(rune('0'+index)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := service.DeleteMessage("message-1")
	if err != nil || !deleted.Deleted || deleted.MessageID != "message-1" {
		t.Fatalf("delete result=%+v err=%v", deleted, err)
	}
	encoded := fmt.Sprintf("%+v", deleted)
	if strings.Contains(encoded, "private-key") || strings.Contains(encoded, "$0") {
		t.Fatalf("mutation result contains private metadata: %s", encoded)
	}
	cleared, err := service.ClearMessages()
	if err != nil || cleared.DeletedCount != 1 || cleared.UnreadCount != 0 {
		t.Fatalf("clear result=%+v err=%v", cleared, err)
	}
}

type testIDs struct{ next int }

func (g *testIDs) NewID() (string, error) {
	g.next++
	return "message-" + string(rune('0'+g.next)), nil
}

type testNotifier struct{ records []domain.MessageRecord }

func (n *testNotifier) Notify(record domain.MessageRecord) { n.records = append(n.records, record) }

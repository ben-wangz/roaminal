package messages

import (
	"errors"
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
	service := New(persistence.NewRepositories(store).Messages, ids)
	now := time.Now().UTC()
	draft := domain.MessageDraft{
		Kind: "codex_turn_completed", Severity: "success", AgentType: "codex", PresentationKey: "codex_turn_finished",
		OccurredAt: now, ReceivedAt: now, EndpointKey: "private-key", FallbackLabel: "fallback",
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
	page, err := service.List(1, "")
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Text != "Codex reporting connected" || page.NextCursor == "" {
		t.Fatalf("first page: %+v err=%v", page, err)
	}
	if page.Messages[0].ConnectionInstanceIDs == nil {
		t.Fatal("message connection instance ids must encode as an empty array")
	}
	older, err := service.List(1, page.NextCursor)
	if err != nil || len(older.Messages) != 1 || older.Messages[0].Text != "Codex turn finished" || older.NextCursor != "" {
		t.Fatalf("older page: %+v err=%v", older, err)
	}
	if _, err := service.List(1, "invalid"); !errors.Is(err, ErrCursorInvalid) {
		t.Fatalf("invalid cursor error=%v", err)
	}
}

type testIDs struct{ next int }

func (g *testIDs) NewID() (string, error) {
	g.next++
	return "message-" + string(rune('0'+g.next)), nil
}

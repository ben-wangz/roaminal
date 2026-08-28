package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestAcceptEventCreatesDurableMessagesForReadyAndStop(t *testing.T) {
	service, token := eventTestService(t)
	if err := service.store.Update("endpoint-test", func(record *EndpointRecord) error {
		record.Aliases = []string{"pve-roaminal"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sink := &recordingMessageAppender{}
	service.messages = sink
	service.terms = &messageConnectionService{views: []ports.ConnectionInstanceView{
		{ID: "instance-a", ConnectionInstanceID: "instance-a", Lifecycle: "live", TmuxSessionName: "roaminal"},
		{ID: "instance-b", ConnectionInstanceID: "instance-b", Lifecycle: "live", TmuxSessionName: "roaminal"},
	}}
	service.bindings["instance-a"] = Target{EndpointKey: "endpoint-test", SessionName: "roaminal"}
	service.bindings["instance-b"] = Target{EndpointKey: "endpoint-test", SessionName: "roaminal"}
	makeEvent := func(name, activity string, sequence uint64) []byte {
		event := webhookEvent{
			SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: name,
			Activity: activity, Sequence: sequence, OccurredAt: time.Now().UTC(),
			Tmux:  webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
			Codex: webhookCodex{SessionID: "codex", TurnID: "turn-1"},
		}
		event.EventID = eventID(event, "endpoint-test")
		body, _ := json.Marshal(event)
		return body
	}
	if duplicate, err := service.AcceptEvent(token, makeEvent("PreToolUse", "running", 1)); err != nil || duplicate {
		t.Fatalf("ready event: duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := service.AcceptEvent(token, makeEvent("Stop", "completed", 2)); err != nil || duplicate {
		t.Fatalf("stop event: duplicate=%v err=%v", duplicate, err)
	}
	if len(sink.records) != 2 || sink.records[0].Kind != "agent_reporting_ready" || sink.records[1].Kind != "codex_turn_completed" {
		t.Fatalf("unexpected messages: %+v", sink.records)
	}
	if got := len(sink.records[0].ConnectionInstanceIDs); got != 2 {
		t.Fatalf("shared target was not attributed to both instances: %d", got)
	}
	if sink.records[0].ConnectionLabel != "pve-roaminal" || sink.records[1].ConnectionLabel != "pve-roaminal" {
		t.Fatalf("unexpected safe connection label: %+v", sink.records)
	}
	if duplicate, err := service.AcceptEvent(token, makeEvent("Stop", "completed", 2)); err != nil || !duplicate {
		t.Fatalf("replayed stop: duplicate=%v err=%v", duplicate, err)
	}
	if len(sink.records) != 2 {
		t.Fatalf("replayed stop created another message: %+v", sink.records)
	}
}

func TestAcceptEventRetriesWhenMessageAppendFails(t *testing.T) {
	service, token := eventTestService(t)
	sink := &recordingMessageAppender{fail: true}
	service.messages = sink
	event := webhookEvent{
		SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: "PreToolUse",
		Activity: "running", Sequence: 1, OccurredAt: time.Now().UTC(),
		Tmux:  webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
		Codex: webhookCodex{SessionID: "codex"},
	}
	event.EventID = eventID(event, "endpoint-test")
	body, _ := json.Marshal(event)
	if duplicate, err := service.AcceptEvent(token, body); duplicate || err == nil {
		t.Fatalf("expected retryable message failure: duplicate=%v err=%v", duplicate, err)
	} else {
		var agentErr *Error
		if !errors.As(err, &agentErr) || agentErr.Code != "message_store_unavailable" || agentErr.Status != 503 {
			t.Fatalf("unexpected message failure: %v", err)
		}
	}
	if _, ok := service.runtimeState(Target{EndpointKey: "endpoint-test", SessionName: "roaminal"}); ok {
		t.Fatal("failed message append committed Agent state")
	}
	sink.fail = false
	if duplicate, err := service.AcceptEvent(token, body); duplicate || err != nil {
		t.Fatalf("retry event: duplicate=%v err=%v", duplicate, err)
	}
	if len(sink.records) != 1 {
		t.Fatalf("retry did not append exactly one message: %+v", sink.records)
	}
}

func TestAcceptEventSerializesConcurrentDuplicateReports(t *testing.T) {
	service, token := eventTestService(t)
	sink := &recordingMessageAppender{}
	service.messages = sink
	event := webhookEvent{
		SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: "PreToolUse",
		Activity: "running", Sequence: 1, OccurredAt: time.Now().UTC(),
		Tmux:  webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
		Codex: webhookCodex{SessionID: "codex"},
	}
	event.EventID = eventID(event, "endpoint-test")
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan struct {
		duplicate bool
		err       error
	}, 2)
	var group sync.WaitGroup
	group.Add(2)
	for range 2 {
		go func() {
			defer group.Done()
			duplicate, err := service.AcceptEvent(token, body)
			results <- struct {
				duplicate bool
				err       error
			}{duplicate: duplicate, err: err}
		}()
	}
	group.Wait()
	close(results)
	accepted, duplicates := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent event error: %v", result.err)
		}
		if result.duplicate {
			duplicates++
		} else {
			accepted++
		}
	}
	if accepted != 1 || duplicates != 1 || len(sink.records) != 1 {
		t.Fatalf("concurrent duplicate handling: accepted=%d duplicates=%d messages=%d", accepted, duplicates, len(sink.records))
	}
}

type recordingMessageAppender struct {
	mu      sync.Mutex
	records []domain.MessageRecord
	fail    bool
}

func (a *recordingMessageAppender) AppendMessage(draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail {
		return domain.MessageRecord{}, false, errors.New("test message store failure")
	}
	for _, record := range a.records {
		if record.IdempotencyKey == draft.IdempotencyKey {
			return record, true, nil
		}
	}
	record := domain.MessageRecord{MessageDraft: draft, Sequence: uint64(len(a.records) + 1)}
	record.ConnectionInstanceIDs = append([]string(nil), draft.ConnectionInstanceIDs...)
	a.records = append(a.records, record)
	return record, false, nil
}

type messageConnectionService struct {
	views []ports.ConnectionInstanceView
}

func (s *messageConnectionService) ConnectionInstanceViews() []ports.ConnectionInstanceView {
	return s.views
}
func (s *messageConnectionService) RemoteTransferInfo(string) (ports.RemoteTransferInfo, error) {
	return ports.RemoteTransferInfo{}, nil
}
func (s *messageConnectionService) RunRemote(context.Context, string, ports.RemoteCommand) (ports.RemoteResult, error) {
	return ports.RemoteResult{}, nil
}
func (s *messageConnectionService) ResolveEndpoint(context.Context, string) (ports.EffectiveEndpoint, error) {
	return ports.EffectiveEndpoint{}, nil
}

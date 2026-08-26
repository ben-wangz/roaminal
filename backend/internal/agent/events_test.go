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

func TestAcceptEventDeduplicatesSequence(t *testing.T) {
	service := NewStoreTestService(t)
	token, hash, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	key := "endpoint-test"
	if err := service.store.Update(key, func(record *EndpointRecord) error {
		record.ActiveTokenHash = hash
		record.InstallationState = "needs_trust"
		record.Targets = map[string]TargetState{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	event := webhookEvent{
		SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: "PreToolUse",
		Activity: "running", Sequence: 1, OccurredAt: time.Now().UTC(),
		Tmux:  webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10},
		Codex: webhookCodex{SessionID: "codex"},
	}
	event.Tmux.PaneID = "%0"
	event.Tmux.SocketFingerprint = "0123456789abcdef"
	event.EventID = eventID(event, key)
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := service.AcceptEvent(token, body)
	if err != nil || duplicate {
		t.Fatalf("first event: duplicate=%v err=%v", duplicate, err)
	}
	duplicate, err = service.AcceptEvent(token, body)
	if err != nil || !duplicate {
		t.Fatalf("duplicate event: duplicate=%v err=%v", duplicate, err)
	}
	record, ok := service.store.Get(key)
	state, stateOK := service.runtimeState(Target{EndpointKey: key, SessionName: "roaminal"})
	if !ok || !stateOK || state.Activity != "running" || state.Component != "ready" || record.InstallationState != "ready" {
		t.Fatalf("unexpected state before runtime loss: %+v", record)
	}
	service.mu.Lock()
	service.runtime = map[string]TargetState{}
	service.runtimeTargets = map[string]string{}
	service.mu.Unlock()
	duplicate, err = service.AcceptEvent(token, body)
	if err != nil || !duplicate {
		t.Fatalf("replayed event after runtime loss: duplicate=%v err=%v", duplicate, err)
	}
}

func TestAcceptEventRejectsClientActivityMismatch(t *testing.T) {
	service := NewStoreTestService(t)
	token, hash, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.store.Update("endpoint-test", func(record *EndpointRecord) error { record.ActiveTokenHash = hash; return nil }); err != nil {
		t.Fatal(err)
	}
	event := webhookEvent{
		SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: "PermissionRequest",
		Activity: "running", Sequence: 1, OccurredAt: time.Now().UTC(),
		Tmux: webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10},
	}
	event.Tmux.PaneID = "%0"
	event.Tmux.SocketFingerprint = "0123456789abcdef"
	event.EventID = eventID(event, "endpoint-test")
	body, _ := json.Marshal(event)
	if _, err := service.AcceptEvent(token, body); err == nil {
		t.Fatal("expected activity mismatch")
	}
}

func TestAcceptEventDoesNotReopenStoppedTurn(t *testing.T) {
	service, token := eventTestService(t)
	makeEvent := func(name, activity, turn string, sequence uint64) []byte {
		event := webhookEvent{
			SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: name,
			Activity: activity, Sequence: sequence, OccurredAt: time.Now().UTC(),
			Tmux:  webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
			Codex: webhookCodex{SessionID: "codex", TurnID: turn},
		}
		event.EventID = eventID(event, "endpoint-test")
		body, _ := json.Marshal(event)
		return body
	}
	for _, input := range [][]byte{
		makeEvent("UserPromptSubmit", "running", "turn-1", 1),
		makeEvent("Stop", "completed", "turn-1", 2),
		makeEvent("UserPromptSubmit", "running", "turn-2", 3),
	} {
		if duplicate, err := service.AcceptEvent(token, input); err != nil || duplicate {
			t.Fatalf("expected accepted event: duplicate=%v err=%v", duplicate, err)
		}
	}
	duplicate, err := service.AcceptEvent(token, makeEvent("PreToolUse", "running", "turn-1", 4))
	if err != nil || !duplicate {
		t.Fatalf("expected late stopped-turn event to be duplicate: duplicate=%v err=%v", duplicate, err)
	}
}

func TestAcceptEventKeepsStopTerminalUntilNewTurn(t *testing.T) {
	service, token := eventTestService(t)
	makeEvent := func(name, activity, source string, sequence uint64) []byte {
		event := webhookEvent{
			SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: name,
			Activity: activity, Sequence: sequence, OccurredAt: time.Now().UTC(),
			Tmux:  webhookTmux{SessionName: "roaminal", SessionID: "$0", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
			Codex: webhookCodex{SessionID: "codex", TurnID: "turn-1"}, Event: webhookSource{Source: source},
		}
		if name == "SessionEnd" {
			event.Event = webhookSource{Reason: "other"}
		}
		if name == "SessionStart" || name == "SessionEnd" {
			event.Codex.TurnID = ""
		}
		event.EventID = eventID(event, "endpoint-test")
		body, _ := json.Marshal(event)
		return body
	}
	for _, input := range [][]byte{
		makeEvent("UserPromptSubmit", "running", "", 1),
		makeEvent("Stop", "completed", "", 2),
	} {
		if duplicate, err := service.AcceptEvent(token, input); err != nil || duplicate {
			t.Fatalf("expected accepted event: duplicate=%v err=%v", duplicate, err)
		}
	}
	if duplicate, err := service.AcceptEvent(token, makeEvent("SessionStart", "running", "compact", 3)); err != nil || !duplicate {
		t.Fatalf("expected compact event to remain terminal: duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := service.AcceptEvent(token, makeEvent("SessionEnd", "idle", "", 4)); err != nil || duplicate {
		t.Fatalf("expected session end after stop: duplicate=%v err=%v", duplicate, err)
	}
}

func TestAcceptEventAcceptsNewTmuxRuntimeSequence(t *testing.T) {
	service, token := eventTestService(t)
	makeEvent := func(sessionID string, created int64) []byte {
		event := webhookEvent{
			SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: "UserPromptSubmit",
			Activity: "running", Sequence: 1, OccurredAt: time.Now().UTC(),
			Tmux:  webhookTmux{SessionName: "roaminal", SessionID: sessionID, SessionCreated: created, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
			Codex: webhookCodex{SessionID: "codex", TurnID: sessionID},
		}
		event.EventID = eventID(event, "endpoint-test")
		body, _ := json.Marshal(event)
		return body
	}
	for _, sessionID := range []string{"$0", "$1"} {
		if duplicate, err := service.AcceptEvent(token, makeEvent(sessionID, 10+int64(len(sessionID)))); err != nil || duplicate {
			t.Fatalf("expected new runtime event: duplicate=%v err=%v", duplicate, err)
		}
	}
}

func TestAcceptEventResolvesRenamedTmuxSession(t *testing.T) {
	service, token := eventTestService(t)
	makeEvent := func(name, eventName, activity string, sequence uint64) []byte {
		event := webhookEvent{
			SchemaVersion: 1, AgentType: "codex", ComponentVersion: "1", EventName: eventName,
			Activity: activity, Sequence: sequence, OccurredAt: time.Now().UTC(),
			Tmux:  webhookTmux{SessionName: name, SessionID: "$0", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"},
			Codex: webhookCodex{SessionID: "codex", TurnID: "turn-1"},
		}
		event.EventID = eventID(event, "endpoint-test")
		body, _ := json.Marshal(event)
		return body
	}
	for _, input := range [][]byte{
		makeEvent("roaminal", "UserPromptSubmit", "running", 1),
		makeEvent("renamed", "Stop", "completed", 2),
	} {
		if duplicate, err := service.AcceptEvent(token, input); err != nil || duplicate {
			t.Fatalf("expected accepted event: duplicate=%v err=%v", duplicate, err)
		}
	}
	state, ok := service.runtimeState(Target{EndpointKey: "endpoint-test", SessionName: "roaminal"})
	if !ok || state.LastEventName != "Stop" {
		t.Fatalf("renamed runtime did not retain original target: ok=%v state=%+v", ok, state)
	}
	if _, ok := service.runtimeState(Target{EndpointKey: "endpoint-test", SessionName: "renamed"}); ok {
		t.Fatal("renamed runtime created a second target")
	}
}

func eventTestService(t *testing.T) (*Service, string) {
	t.Helper()
	service := NewStoreTestService(t)
	token, hash, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.store.Update("endpoint-test", func(record *EndpointRecord) error { record.ActiveTokenHash = hash; return nil }); err != nil {
		t.Fatal(err)
	}
	return service, token
}

func NewStoreTestService(t *testing.T) *Service {
	t.Helper()
	return &Service{store: OpenStore(t.TempDir()), bindings: map[string]Target{}, operations: map[string]*Initialization{}, endpointOps: map[string]string{}, endpointLock: map[string]*sync.Mutex{}, runtime: map[string]TargetState{}, runtimeTargets: map[string]string{}, completedTools: map[string]map[string]time.Time{}, eventIDs: map[string]map[string]time.Time{}, endpointCache: map[string]endpointCacheEntry{}}
}

func TestAcceptEventCreatesDurableMessagesForReadyAndStop(t *testing.T) {
	service, token := eventTestService(t)
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

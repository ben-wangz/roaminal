package agent

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
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

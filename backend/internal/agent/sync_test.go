package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type syncConnectionService struct {
	views      []ports.ConnectionInstanceView
	endpoints  map[string]ports.EffectiveEndpoint
	transfers  map[string]error
	remote     map[string]syncRemoteResult
	remoteCall []string
	mu         sync.Mutex
}

type syncRemoteResult struct {
	result ports.RemoteResult
	err    error
}

func (s *syncConnectionService) ConnectionInstanceViews() []ports.ConnectionInstanceView {
	return append([]ports.ConnectionInstanceView(nil), s.views...)
}

func (s *syncConnectionService) RemoteTransferInfo(id string) (ports.RemoteTransferInfo, error) {
	if err := s.transfers[id]; err != nil {
		return ports.RemoteTransferInfo{}, err
	}
	return ports.RemoteTransferInfo{Alias: id, ControlPath: "/tmp/" + id, SSHPath: "ssh"}, nil
}

func (s *syncConnectionService) RunRemote(_ context.Context, id string, _ ports.RemoteCommand) (ports.RemoteResult, error) {
	s.mu.Lock()
	s.remoteCall = append(s.remoteCall, id)
	s.mu.Unlock()
	value, ok := s.remote[id]
	if !ok {
		return ports.RemoteResult{}, errors.New("remote result missing")
	}
	return value.result, value.err
}

func (s *syncConnectionService) ResolveEndpoint(_ context.Context, id string) (ports.EffectiveEndpoint, error) {
	value, ok := s.endpoints[id]
	if !ok {
		return ports.EffectiveEndpoint{}, errors.New("endpoint missing")
	}
	return value, nil
}

type syncMessageAppender struct {
	mu      sync.Mutex
	records []domain.MessageRecord
}

func (a *syncMessageAppender) AppendMessage(draft domain.MessageDraft) (domain.MessageRecord, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, record := range a.records {
		if record.IdempotencyKey == draft.IdempotencyKey {
			return record, true, nil
		}
	}
	record := domain.MessageRecord{MessageDraft: draft, Sequence: uint64(len(a.records) + 1)}
	a.records = append(a.records, record)
	return record, false, nil
}

func syncView(id, definitionID, session string) ports.ConnectionInstanceView {
	alias := "remote-" + id
	return ports.ConnectionInstanceView{
		ID: id, ConnectionInstanceID: id, ConnectionDefinitionID: definitionID,
		Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias,
		TmuxEnabled: true, TmuxSessionName: session,
	}
}

func syncSnapshot(session, sessionID string, created int64, socket, state string, index uint64) remoteAgentState {
	identity := remoteTmuxIdentity{SessionName: session, SessionID: sessionID, SessionCreated: created, PaneID: "%0", SocketFingerprint: socket}
	when := time.Unix(int64(index), 0).UTC()
	return remoteAgentState{
		SchemaVersion: 1, Provider: "codex", ComponentVersion: "1",
		Capabilities: remoteCapabilities{Running: true, Relax: true, Error: false},
		Tmux:         identity, RuntimeID: runtimeID(identity), State: state, LatestIndex: index,
		Records: []remoteStateRecord{{Index: index, Timestamp: when, State: state, EventName: "hook"}}, UpdatedAt: when,
	}
}

func encodeSyncSnapshot(t *testing.T, snapshot remoteAgentState) []byte {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newSyncService(t *testing.T, terms *syncConnectionService, messages *syncMessageAppender) *Service {
	t.Helper()
	return NewWithRepository(config.Config{}, OpenStore(t.TempDir()), terms, Dependencies{Messages: messages, SyncInterval: time.Hour})
}

func TestSyncOnceFiltersConnectionInstancesAndContinuesAfterFailure(t *testing.T) {
	good := syncView("good", "definition-good", "team")
	failed := syncView("failed", "definition-failed", "team")
	local := good
	local.ID = "local"
	local.Type = "local"
	dead := good
	dead.ID = "dead"
	dead.Lifecycle = "closed"
	noTmux := good
	noTmux.ID = "no-tmux"
	noTmux.TmuxEnabled = false
	terms := &syncConnectionService{
		views: []ports.ConnectionInstanceView{failed, local, dead, noTmux, good},
		endpoints: map[string]ports.EffectiveEndpoint{
			"failed": {User: "coder", Host: "failed.test", Port: 22},
			"good":   {User: "coder", Host: "good.test", Port: 22},
		},
		remote: map[string]syncRemoteResult{
			"failed": {err: errors.New("remote state unavailable")},
			"good":   {result: ports.RemoteResult{Output: encodeSyncSnapshot(t, syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1))}},
		},
	}
	service := newSyncService(t, terms, nil)
	service.SyncOnce(context.Background())
	if len(terms.remoteCall) != 2 || terms.remoteCall[0] != "failed" || terms.remoteCall[1] != "good" {
		t.Fatalf("remote calls = %#v, want only eligible SSH/tmux instances in order", terms.remoteCall)
	}
	endpoint, err := NormalizeEndpoint(terms.endpoints["good"])
	if err != nil {
		t.Fatal(err)
	}
	record, ok := service.store.Get(endpoint.Key)
	if !ok || record.Targets["team"].State != "running" || record.Targets["team"].SyncStatus != syncStatusAvailable {
		t.Fatalf("good instance was not synchronized: ok=%v record=%+v", ok, record)
	}
}

func TestAcceptSnapshotCreatesOnlyActualTransitionsAndDeduplicatesIndex(t *testing.T) {
	view := syncView("instance", "definition", "team")
	terms := &syncConnectionService{views: []ports.ConnectionInstanceView{view}}
	messages := &syncMessageAppender{}
	service := newSyncService(t, terms, messages)
	endpoint, err := NormalizeEndpoint(ports.EffectiveEndpoint{User: "coder", Host: "host.test", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	target := Target{EndpointKey: endpoint.Key, SessionName: "team"}
	service.bindings[view.ID] = target
	if err := service.store.Update(endpoint.Key, func(record *EndpointRecord) error {
		record.User, record.Host, record.Port = endpoint.User, endpoint.Host, endpoint.Port
		record.Aliases = []string{"remote-instance"}
		record.Targets = map[string]TargetState{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info := func(state string, index uint64) remoteAgentState {
		return syncSnapshot("team", "$1", 10, "0123456789abcdef", state, index)
	}
	for _, snapshot := range []remoteAgentState{info("relax", 1), info("running", 2), info("relax", 3), info("relax", 3)} {
		if err := service.acceptSnapshot(endpoint.Key, target, view, EndpointRecord{}, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if len(messages.records) != 2 {
		t.Fatalf("transition messages = %d, want 2", len(messages.records))
	}
	if messages.records[0].AgentStateFrom != "relax" || messages.records[0].AgentStateTo != "running" || messages.records[1].AgentStateFrom != "running" || messages.records[1].AgentStateTo != "relax" {
		t.Fatalf("unexpected transitions: %+v", messages.records)
	}
	if len(messages.records[1].ConnectionInstanceIDs) != 1 || messages.records[1].ConnectionDefinitionIDs[0] != "definition" {
		t.Fatalf("transition attribution missing: %+v", messages.records[1])
	}
}

func TestAcceptSnapshotRejectsOlderIndexAndOlderRuntime(t *testing.T) {
	view := syncView("instance", "definition", "team")
	terms := &syncConnectionService{views: []ports.ConnectionInstanceView{view}}
	service := newSyncService(t, terms, nil)
	endpoint, err := NormalizeEndpoint(ports.EffectiveEndpoint{User: "coder", Host: "host.test", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	target := Target{EndpointKey: endpoint.Key, SessionName: "team"}
	newer := syncSnapshot("team", "$2", 20, "0123456789abcdef", "running", 4)
	if err := service.acceptSnapshot(endpoint.Key, target, view, EndpointRecord{}, newer); err != nil {
		t.Fatal(err)
	}
	if err := service.acceptSnapshot(endpoint.Key, target, view, EndpointRecord{}, syncSnapshot("team", "$2", 20, "0123456789abcdef", "relax", 3)); !errors.Is(err, errStaleSnapshot) {
		t.Fatalf("older index error = %v, want stale snapshot", err)
	}
	if err := service.acceptSnapshot(endpoint.Key, target, view, EndpointRecord{}, syncSnapshot("team", "$1", 10, "0123456789abcdef", "relax", 99)); !errors.Is(err, errStaleSnapshot) {
		t.Fatalf("older runtime error = %v, want stale snapshot", err)
	}
	record, ok := service.store.Get(endpoint.Key)
	if !ok || record.Targets["team"].RuntimeID != newer.RuntimeID || record.Targets["team"].StateIndex != 4 || record.Targets["team"].SyncStatus != syncStatusStale {
		t.Fatalf("stale snapshot replaced projection: ok=%v record=%+v", ok, record)
	}
}

func TestAcceptSnapshotRejectsConflictingStateAtSameIndex(t *testing.T) {
	view := syncView("instance", "definition", "team")
	terms := &syncConnectionService{views: []ports.ConnectionInstanceView{view}}
	service := newSyncService(t, terms, nil)
	endpoint, err := NormalizeEndpoint(ports.EffectiveEndpoint{User: "coder", Host: "host.test", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	target := Target{EndpointKey: endpoint.Key, SessionName: "team"}
	initial := syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 2)
	if err := service.acceptSnapshot(endpoint.Key, target, view, EndpointRecord{}, initial); err != nil {
		t.Fatal(err)
	}
	conflicting := syncSnapshot("team", "$1", 10, "0123456789abcdef", "relax", 2)
	if err := service.acceptSnapshot(endpoint.Key, target, view, EndpointRecord{}, conflicting); !errors.Is(err, errStaleSnapshot) {
		t.Fatalf("same-index conflicting state error = %v, want stale snapshot", err)
	}
	record, ok := service.store.Get(endpoint.Key)
	if !ok || record.Targets["team"].State != "running" || record.Targets["team"].SyncStatus != syncStatusStale {
		t.Fatalf("conflicting snapshot changed projection: ok=%v record=%+v", ok, record)
	}
}

func TestValidateSnapshotRequiresDeclaredCapabilities(t *testing.T) {
	snapshot := syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1)
	snapshot.Capabilities.Running = false
	if err := validateSnapshot(snapshot, "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("unsupported running capability error = %v", err)
	}
	snapshot = syncSnapshot("team", "$1", 10, "0123456789abcdef", "error", 1)
	snapshot.Capabilities.Error = false
	if err := validateSnapshot(snapshot, "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("unsupported error capability error = %v", err)
	}
	snapshot = syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1)
	snapshot.Records[0].Source = "startup"
	snapshot.Records[0].Reason = "user_requested"
	snapshot.Records[0].ProviderSessionID = "provider-session"
	snapshot.Records[0].TurnID = "turn"
	snapshot.Records[0].ToolUseID = "tool"
	if err := validateSnapshot(snapshot, "team"); err != nil {
		t.Fatalf("safe provider metadata was rejected: %v", err)
	}
	snapshot.Records[0].ToolUseID = strings.Repeat("x", 129)
	if err := validateSnapshot(snapshot, "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("oversized provider metadata error = %v", err)
	}
}

func TestReadRemoteStateRejectsTrailingJSON(t *testing.T) {
	view := syncView("instance", "definition", "team")
	output := append(
		encodeSyncSnapshot(t, syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1)),
		[]byte(`{"extra":true}`)...,
	)
	terms := &syncConnectionService{
		views: []ports.ConnectionInstanceView{view},
		remote: map[string]syncRemoteResult{
			"instance": {result: ports.RemoteResult{Output: output}},
		},
	}
	service := newSyncService(t, terms, nil)
	if _, err := service.readRemoteState(context.Background(), "instance", "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("trailing JSON error = %v, want invalid state", err)
	}
}

func TestSyncClassifiesMissingTmuxSessionSeparately(t *testing.T) {
	view := syncView("instance", "definition", "team")
	terms := &syncConnectionService{
		views:     []ports.ConnectionInstanceView{view},
		endpoints: map[string]ports.EffectiveEndpoint{"instance": {User: "coder", Host: "host.test", Port: 22}},
		remote: map[string]syncRemoteResult{
			"instance": {result: ports.RemoteResult{ErrorOutput: []byte(`{"error":"tmux session unavailable","code":"tmux_session_missing"}`)}, err: errors.New("remote command failed")},
		},
	}
	service := newSyncService(t, terms, nil)
	service.SyncOnce(context.Background())
	endpoint, err := NormalizeEndpoint(terms.endpoints["instance"])
	if err != nil {
		t.Fatal(err)
	}
	record, ok := service.store.Get(endpoint.Key)
	if !ok || record.Targets["team"].SyncStatus != syncStatusTmuxMissing {
		t.Fatalf("missing tmux session was not classified separately: ok=%v record=%+v", ok, record)
	}
}

func TestSafeSyncErrorIsBounded(t *testing.T) {
	message := safeSyncError(errors.New(fmt.Sprintf("%01000d", 1)))
	if len(message) != 256 {
		t.Fatalf("safe sync error length = %d, want 256", len(message))
	}
}

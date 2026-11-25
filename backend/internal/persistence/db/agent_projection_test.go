package db

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/messages"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"gorm.io/gorm"
)

func TestSQLiteAgentProjectionTransaction(t *testing.T) {
	s, c := sqliteStore(t)
	testAgentProjectionTransaction(t, s, reopenMessageNotificationStore(t, c))
}

func TestPostgresAgentProjectionTransaction(t *testing.T) {
	s, c := postgresStore(t)
	testAgentProjectionTransaction(t, s, reopenMessageNotificationStore(t, c))
}

func testAgentProjectionTransaction(t *testing.T, s, reader *Store) {
	t.Helper()
	key, _ := agentEndpointFixture(t)
	checkpoints := reader.AgentRepository()
	var notifications []domain.MessageRecord
	notifier := agentProjectionNotifier(func(record domain.MessageRecord) {
		notifications = append(notifications, record)
		endpoint, ok := checkpoints.Get(key)
		state := endpoint.Targets["work"]
		if !ok || state.StateIndex != record.AgentStateIndex || state.LastTransitionKey != record.IdempotencyKey {
			t.Errorf("notification ran before checkpoint commit: state = %+v, record = %+v", state, record)
			return
		}
		page, err := reader.ListMessages(1, 0)
		if err != nil || len(page.Messages) != 1 || page.Messages[0].MessageID != record.MessageID {
			t.Errorf("notification ran before message commit: page = %+v, error = %v", page, err)
		}
	})
	source := &agentProjectionSource{state: "running", index: 1}
	service := newAgentProjectionService(s, source, notifier)
	sync := func() { service.SyncOnce(context.Background()) }
	sync()
	initial, ok := checkpoints.Get(key)
	if !ok || initial.Targets["work"].StateIndex != 1 || len(notifications) != 0 {
		t.Fatalf("initial checkpoint = %+v, notifications = %d", initial, len(notifications))
	}
	seed := seedAgentProjectionMessages(t, s)
	source.state, source.index = "relax", 2

	// Fail after either SQL UPDATE has executed. The transaction must undo
	// message insertion, retention pruning, and singleton cursor changes.
	var failTable string
	var failures int
	if err := s.DB.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("test:reject_agent_projection", func(tx *gorm.DB) {
		if failTable != "" && tx.Statement.Table == failTable && tx.Error == nil && tx.RowsAffected == 1 {
			failures++
			_ = tx.AddError(errors.New("injected projection write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	for i, table := range []string{"message_state", "agent_endpoints"} {
		failTable = table
		sync()
		if failures != i+1 {
			t.Fatalf("failed to exercise %s write", table)
		}
		if got, ok := checkpoints.Get(key); !ok || !reflect.DeepEqual(got, initial) {
			t.Fatalf("failed %s write changed checkpoint: %+v", table, got)
		}
		assertAgentProjectionMessages(t, reader, seed, 2, 1)
		if len(notifications) != 0 {
			t.Fatalf("failed %s write notified %d times", table, len(notifications))
		}
	}
	failTable = ""
	sync()
	sync() // Same checkpoint: no append or push.
	assertAgentProjectionMessages(t, reader, domain.MessageState{LatestSequence: 3, Revision: 6, ReadThroughSequence: 1, UnreadCount: 2}, 2, 2)
	if len(notifications) != 1 || notifications[0].Sequence != 3 || notifications[0].AgentStateFrom != "running" || notifications[0].AgentStateTo != "relax" {
		t.Fatalf("retry notifications = %+v", notifications)
	}
	if _, deleted, err := s.DeleteMessage(notifications[0].MessageID); err != nil || !deleted {
		t.Fatalf("delete transition: %v, %v", deleted, err)
	}
	sync() // The transition key is no longer retained by the message store.
	assertAgentProjectionMessages(t, reader, domain.MessageState{LatestSequence: 3, Revision: 7, ReadThroughSequence: 1, UnreadCount: 1}, 1, 2)
	if len(notifications) != 1 {
		t.Fatalf("retry after deletion sent %d notifications, want one total", len(notifications))
	}

	// A message left by the pre-transaction implementation is a duplicate,
	// but must not prevent committing its missing checkpoint.
	orphan := notifications[0].MessageDraft
	orphan.MessageID += "-orphan"
	orphan.IdempotencyKey = projectionRuntimeID + "\x003"
	orphan.AgentStateFrom, orphan.AgentStateTo, orphan.AgentStateIndex = "relax", "running", 3
	if _, duplicate, err := s.AppendMessage(orphan); err != nil || duplicate {
		t.Fatalf("seed existing message: duplicate = %t, err = %v", duplicate, err)
	}
	source.state, source.index = "running", 3
	sync()
	assertAgentProjectionMessages(t, reader, domain.MessageState{LatestSequence: 4, Revision: 8, ReadThroughSequence: 1, UnreadCount: 2}, 2, 2)
	if got, ok := checkpoints.Get(key); !ok || got.Targets["work"].StateIndex != 3 || got.Targets["work"].LastTransitionKey != orphan.IdempotencyKey || len(notifications) != 1 {
		t.Fatalf("duplicate did not advance checkpoint without a push: endpoint = %+v, notifications = %d", got, len(notifications))
	}

	testConcurrentAgentSnapshots(t, s, reader, key)
}

func testConcurrentAgentSnapshots(t *testing.T, s, other *Store, key string) {
	t.Helper()
	// Separate services do not share the in-process endpoint mutex. Either
	// index can commit first, but only one can transition away from running.
	var pushes atomic.Int32
	notifier := agentProjectionNotifier(func(domain.MessageRecord) { pushes.Add(1) })
	older := newAgentProjectionService(s, &agentProjectionSource{state: "relax", index: 4}, notifier)
	newer := newAgentProjectionService(other, &agentProjectionSource{state: "relax", index: 5}, notifier)
	services := []*agent.Service{older, newer}
	runConcurrentRepositoryCalls(len(services), func(i int) { services[i].SyncOnce(context.Background()) })
	older.SyncOnce(context.Background()) // A stale read only changes sync metadata.
	endpoint, ok := other.AgentRepository().Get(key)
	if state := endpoint.Targets["work"]; !ok || state.StateIndex != 5 || state.State != "relax" || state.SyncStatus != "stale" || pushes.Load() != 1 {
		t.Fatalf("concurrent checkpoint = %+v, notification count = %d", state, pushes.Load())
	}
	assertAgentProjectionMessages(t, other, domain.MessageState{LatestSequence: 5, Revision: 9, ReadThroughSequence: 1, UnreadCount: 3}, 3, 2)
}

func seedAgentProjectionMessages(t *testing.T, s *Store) domain.MessageState {
	t.Helper()
	now := time.Now().UTC()
	seed := domain.MessageState{LatestSequence: 2, Revision: 5, ReadThroughSequence: 1, UnreadCount: 1}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		return importMessages(tx, seed, []domain.MessageRecord{
			{Sequence: 1, MessageDraft: messageDraft(1, now.Add(-8*24*time.Hour))},
			{Sequence: 2, MessageDraft: messageDraft(2, now)},
		})
	}); err != nil {
		t.Fatal(err)
	}
	return seed
}

func assertAgentProjectionMessages(t *testing.T, s *Store, want domain.MessageState, count int, oldest uint64) {
	t.Helper()
	state, err := s.MessageState()
	if err != nil || state != want {
		t.Fatalf("message state = %+v, %v; want %+v", state, err, want)
	}
	page, err := s.ListMessages(100, 0)
	if err != nil || len(page.Messages) != count || page.Messages[count-1].Sequence != oldest {
		t.Fatalf("message page = %+v, %v; want %d records back to sequence %d", page, err, count, oldest)
	}
}

func newAgentProjectionService(s *Store, source *agentProjectionSource, notifier ports.MessageNotifier) *agent.Service {
	ids := identity.UUIDGenerator{Random: random.CryptoSource{}}
	messageService := messages.NewWithNotifier(s, ids, notifier)
	return agent.NewWithRepository(config.Config{}, s.AgentRepository(), source, agent.Dependencies{Messages: messageService})
}

type agentProjectionNotifier func(domain.MessageRecord)

func (f agentProjectionNotifier) Notify(record domain.MessageRecord) { f(record) }

type agentProjectionSource struct {
	state string
	index uint64
}

func (*agentProjectionSource) ConnectionInstanceViews() []ports.ConnectionInstanceView {
	return []ports.ConnectionInstanceView{{ID: "instance", ConnectionInstanceID: "instance", Type: "ssh", Lifecycle: "live", TmuxEnabled: true, TmuxSessionName: "work"}}
}

func (*agentProjectionSource) ResolveEndpoint(context.Context, string) (ports.EffectiveEndpoint, error) {
	return ports.EffectiveEndpoint{User: "alice", Host: "example.test", Port: 22}, nil
}

func (*agentProjectionSource) RemoteTransferInfo(string) (ports.RemoteTransferInfo, error) {
	return ports.RemoteTransferInfo{}, nil
}

// The runtime ID is the first 32 hex characters of SHA256("$0|10|0123456789abcdef").
const projectionRuntimeID = "cdee6c1e4426b6e559fd87561a87bdbe"

func (r *agentProjectionSource) RunRemote(context.Context, string, ports.RemoteCommand) (ports.RemoteResult, error) {
	return ports.RemoteResult{Output: fmt.Appendf(nil, `{
		"schemaVersion":1,"provider":"codex","componentVersion":"1","capabilities":{"running":true,"relax":true},
		"tmux":{"sessionName":"work","sessionId":"$0","sessionCreated":10,"paneId":"%%0","socketFingerprint":"0123456789abcdef"},
		"runtimeId":%q,"state":%q,"latestIndex":%d,"updatedAt":"2025-01-01T00:00:00Z",
		"records":[{"index":%d,"state":%q,"timestamp":"2025-01-01T00:00:00Z"}]
	}`, projectionRuntimeID, r.state, r.index, r.index, r.state)}, nil
}

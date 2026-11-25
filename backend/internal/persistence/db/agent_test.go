package db

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func agentEndpointFixture(t *testing.T) (string, domain.AgentEndpointRecord) {
	t.Helper()
	value := domain.AgentEndpointRecord{User: "alice", Host: "example.test", Port: 22, Aliases: []string{"alpha"},
		Targets: map[string]domain.AgentTargetState{"work": {SessionName: "work", State: "running"}}}
	endpoint, err := agent.NormalizeEndpoint(ports.EffectiveEndpoint{User: value.User, Host: value.Host, Port: value.Port})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint.Key, value
}

func TestAgentRepositoryImportUpdateAndReopen(t *testing.T) {
	s, cfg := sqliteStore(t)
	repo := s.AgentRepository()
	key, value := agentEndpointFixture(t)
	if !repo.Available() {
		t.Fatalf("repository unavailable: %v", repo.Err())
	}
	if err := s.ImportAgentEndpoint(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportAgentEndpoint(context.Background(), value); err == nil {
		t.Fatal("offline import overwrote an endpoint")
	}
	stop := errors.New("abort update")
	if err := repo.Update(key, func(record *domain.AgentEndpointRecord) error {
		record.Aliases[0] = "discarded"
		return stop
	}); !errors.Is(err, stop) {
		t.Fatalf("callback error = %v", err)
	}
	if err := repo.Update(key, func(record *domain.AgentEndpointRecord) error {
		record.Targets["work"] = domain.AgentTargetState{SessionName: "work", LastTransitionKey: "runtime\x002"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if view := repo.Snapshot(); len(view) != 1 || view[key].Aliases[0] != "alpha" {
		t.Fatalf("snapshot after callback: %+v", view)
	} else {
		view[key].Aliases[0] = "modified"
		delete(view[key].Targets, "work")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if repo.Err() == nil || repo.Available() {
		t.Fatal("closed repository reported available")
	}
	reopened, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, ok := reopened.AgentRepository().Get(key)
	if !ok || got.Aliases[0] != "alpha" || got.Targets["work"].LastTransitionKey != "runtime\x002" {
		t.Fatalf("reopened endpoint differs: %+v", got)
	}
}

func TestAgentRepositoryConcurrentEndpointUpdates(t *testing.T) {
	s, cfg := sqliteStore(t)
	other, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	key, value := agentEndpointFixture(t)
	const workers = 18
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			store := s
			if i%2 != 0 {
				store = other
			}
			if err := store.AgentRepository().Update(key, func(record *domain.AgentEndpointRecord) error {
				record.User, record.Host, record.Port = value.User, value.Host, value.Port
				state := record.Targets["work"]
				state.SessionName = "work"
				state.StateIndex++
				record.Targets["work"] = state
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	got, ok := s.AgentRepository().Get(key)
	if !ok || got.Targets["work"].StateIndex != workers {
		t.Fatalf("lost updates: %+v", got)
	}
}

func TestAgentRepositoryRejectsInvalidIdentityAndTargets(t *testing.T) {
	s, _ := sqliteStore(t)
	key, value := agentEndpointFixture(t)
	repo := s.AgentRepository()
	if err := repo.Update(key, func(record *domain.AgentEndpointRecord) error {
		*record = value
		record.Port++
		return nil
	}); !errors.Is(err, errAgentRecord) {
		t.Fatalf("invalid binding accepted: %v", err)
	}
	if _, exists := repo.Get(key); exists {
		t.Fatal("failed updated inserted a partial row")
	}
	value.Targets["work"] = domain.AgentTargetState{SessionName: "different"}
	if err := s.ImportAgentEndpoint(context.Background(), value); !errors.Is(err, errAgentRecord) {
		t.Fatalf("invalid target accepted: %v", err)
	}
	value.Targets["work"] = domain.AgentTargetState{SessionName: "work"}
	if err := s.ImportAgentEndpoint(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Model(&agentEndpointRow{Key: key}).Update("format_version", agentEndpointVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if _, exists := repo.Get(key); exists {
		t.Fatal("unsupported record format was returned")
	}
	if err := repo.Update(key, func(*domain.AgentEndpointRecord) error { return nil }); !errors.Is(err, errAgentRecord) {
		t.Fatalf("unsupported record format was replaced: %v", err)
	}
}

func TestAgentImportPreservesPartialEndpointKey(t *testing.T) {
	s, _ := sqliteStore(t)
	key, complete := agentEndpointFixture(t)
	partial := domain.AgentEndpointRecord{Targets: map[string]domain.AgentTargetState{
		"work": {SessionName: "work", SyncStatus: "unavailable"},
	}}
	if err := s.ImportAgentEndpointWithKey(context.Background(), key, partial); err != nil {
		t.Fatal(err)
	}
	repo := s.AgentRepository()
	if got, ok := repo.Get(key); !ok || got.User != "" || got.Targets["work"].SyncStatus != "unavailable" {
		t.Fatalf("partial binding was not preserved: %+v", got)
	}
	if err := repo.Update(key, func(record *domain.AgentEndpointRecord) error {
		record.User, record.Host, record.Port = complete.User, complete.Host, complete.Port
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, ok := repo.Get(key); !ok || got.User != complete.User || got.Targets["work"].SyncStatus != "unavailable" {
		t.Fatalf("populating identity lost the projection: %+v", got)
	}
}

func TestAgentImportObeysCallerTransaction(t *testing.T) {
	cfg := Config{Driver: SQLite, SQLitePath: filepath.Join(t.TempDir(), "private", "agent.sqlite3")}
	s, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key, record := agentEndpointFixture(t)
	tx := s.DB.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := (&Store{DB: tx, driver: s.driver}).ImportAgentEndpoint(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := s.AgentRepository().Get(key); ok {
		t.Fatal("endpoint survived caller transaction rollback")
	}
}

package terminal

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type shutdownWorker struct {
	ports.TerminalWorker
	stopped chan struct{}
}

func (w shutdownWorker) Snapshot(_ context.Context, _ string, sequence string) ([]byte, string, error) {
	return []byte("snapshot " + sequence), sequence, nil
}

func (w shutdownWorker) CloseSession(context.Context, string) error { return nil }
func (w shutdownWorker) Shutdown(context.Context) error {
	close(w.stopped)
	return nil
}

func waitForCleanup[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("terminal cleanup did not finish")
		panic("unreachable")
	}
}

func TestShutdownPreservesLastCommittedCheckpoint(t *testing.T) {
	m, store, meta := retirementFixture(t)
	if err := store.SaveSnapshot(meta.ID, domain.SnapshotHeader{Cols: meta.Cols, Rows: meta.Rows, ThroughSequence: "0"}, []byte("committed")); err != nil {
		t.Fatal(err)
	}
	periodic := blockedPeriodicSnapshot{
		TerminalSnapshotRepository: m.repositories.Snapshots, entered: make(chan struct{}), release: make(chan struct{}),
	}
	t.Cleanup(func() { close(periodic.release) })
	m.repositories.Snapshots = periodic
	worker := shutdownWorker{stopped: make(chan struct{})}
	m.worker = worker
	s := &Session{manager: m, meta: meta, sequence: 1}
	m.sessions[meta.ID] = s
	periodicDone := make(chan error, 1)
	go func() { periodicDone <- s.saveSnapshotWithClosed(t.Context(), false) }()
	waitForCleanup(t, periodic.entered)
	s.mu.Lock()
	s.sequence = 2
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	returned := make(chan struct{})
	go func() { m.Shutdown(ctx); close(returned) }()
	waitForCleanup(t, returned)
	waitForCleanup(t, worker.stopped)
	if err := waitForCleanup(t, periodicDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight snapshot did not cancel: %v", err)
	}
	if _, err := store.LoadConnectionInstance(meta.ID); err != nil {
		t.Fatalf("shutdown removed recoverable active metadata: %v", err)
	}
	if _, payload, err := store.LoadSnapshot(meta.ID); err != nil || string(payload) != "committed" {
		t.Fatalf("shutdown changed the last committed snapshot: %q, %v", payload, err)
	}
	if err := m.saveMetaContext(t.Context(), meta); !errors.Is(err, context.Canceled) {
		t.Fatalf("stopped runtime still admitted a metadata write: %v", err)
	}

	restarted := newTestManager(config.Config{}, store, nil)
	if err := restarted.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadConnectionInstance(meta.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restart did not retire the active connection instance: %v", err)
	}
	data, err := os.ReadFile(store.AuditConnectionSnapshotPath(meta.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, payload, err := persistence.DecodeSnapshot(data); err != nil || string(payload) != "committed" {
		t.Fatalf("restart did not archive the committed snapshot: %q, %v", payload, err)
	}
}

func TestShutdownArchivesHealthyInstance(t *testing.T) {
	m, store, meta := retirementFixture(t)
	worker := shutdownWorker{stopped: make(chan struct{})}
	m.worker = worker
	m.sessions[meta.ID] = &Session{manager: m, meta: meta, sequence: 1}
	m.Shutdown(t.Context())
	waitForCleanup(t, worker.stopped)
	if _, err := store.LoadConnectionInstance(meta.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shutdown left the healthy instance active: %v", err)
	}
	data, err := os.ReadFile(store.AuditConnectionSnapshotPath(meta.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, payload, err := persistence.DecodeSnapshot(data); err != nil || string(payload) != "snapshot 1" {
		t.Fatalf("final snapshot = %q, %v", payload, err)
	}
}

func TestShutdownCancelsAndJoinsEarlierRetirement(t *testing.T) {
	m, store, meta := retirementFixture(t)
	worker := shutdownWorker{stopped: make(chan struct{})}
	m.worker = worker
	s := &Session{manager: m, meta: meta, closed: true}
	m.sessions[meta.ID] = s
	entered, cancelled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(finish) })
	t.Cleanup(release)
	m.repositories.RetireConnectionInstance = func(ctx context.Context, _ domain.ConnectionInstanceID) error {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		// Model a storage driver still finishing rollback after cancellation.
		<-finish
		return ctx.Err()
	}
	retired := make(chan error, 1)
	go func() { retired <- m.retireSession(t.Context(), s) }()
	waitForCleanup(t, entered)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	returned := make(chan struct{})
	go func() { m.Shutdown(ctx); close(returned) }()
	select {
	case <-worker.stopped:
		t.Fatal("shutdown treated an in-flight retirement as already complete")
	case <-time.After(25 * time.Millisecond):
	}
	cancel()
	waitForCleanup(t, cancelled)
	waitForCleanup(t, worker.stopped)
	select {
	case <-returned:
		t.Fatal("shutdown returned before the repository call completed")
	case <-time.After(25 * time.Millisecond):
	}
	release()
	if err := waitForCleanup(t, retired); !errors.Is(err, context.Canceled) {
		t.Fatalf("earlier retirement did not cancel: %v", err)
	}
	waitForCleanup(t, returned)
	if _, err := store.LoadConnectionInstance(meta.ID); err != nil {
		t.Fatalf("cancelled archive removed active metadata: %v", err)
	}
}

type stalledMetadataSave struct {
	ports.ConnectionInstanceRepository
	entered chan struct{}
}

func (r stalledMetadataSave) SaveConnectionInstance(ctx context.Context, _ domain.ConnectionInstanceMeta) error {
	close(r.entered)
	<-ctx.Done()
	return ctx.Err()
}

func TestRetirementDeadlineStopsFinalMetadataWrite(t *testing.T) {
	m, store, meta := retirementFixture(t)
	m.worker = shutdownWorker{}
	repo := stalledMetadataSave{ConnectionInstanceRepository: m.repositories.Instances, entered: make(chan struct{})}
	m.repositories.Instances = repo
	m.repositories.RetireConnectionInstance = func(context.Context, domain.ConnectionInstanceID) error {
		t.Error("retirement attempted an archive after failed metadata")
		return errors.New("unexpected archive")
	}
	s := &Session{manager: m, meta: meta, closed: true, sequence: 1}
	m.sessions[meta.ID] = s
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	retired := make(chan error, 1)
	go func() { retired <- m.retireSession(ctx, s) }()
	waitForCleanup(t, repo.entered)
	if err := waitForCleanup(t, retired); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled metadata did not honor deadline: %v", err)
	}
	if _, err := store.LoadConnectionInstance(meta.ID); err != nil {
		t.Fatal(err)
	}
	if header, _, err := store.LoadSnapshot(meta.ID); err != nil || header.ThroughSequence != "1" {
		t.Fatalf("active final checkpoint lost: %+v, %v", header, err)
	}
}

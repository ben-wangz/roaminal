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

func retirementFixture(t *testing.T) (*Manager, *persistence.Store, domain.ConnectionInstanceMeta) {
	t.Helper()
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := newTestManager(config.Config{}, store, nil)
	now := m.now().UTC()
	meta := domain.ConnectionInstanceMeta{
		ID: "11111111-1111-4111-8111-111111111111", InitialCwd: "/workspace", Cwd: "/workspace",
		Lifecycle: "live", BackendRuntimeID: "previous-runtime", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now,
	}
	if err := m.saveMeta(meta); err != nil {
		t.Fatal(err)
	}
	return m, store, meta
}

type retirementAuditError struct{ err error }

func (a retirementAuditError) ArchiveConnectionInstance(context.Context, domain.ConnectionInstanceID) error {
	return a.err
}

func TestStartupRetirementUsesOptionalCallback(t *testing.T) {
	m, store, meta := retirementFixture(t)
	m.repositories.Audit = retirementAuditError{errors.New("unexpected legacy archive")}
	var checkpoint domain.ConnectionInstanceMeta
	calls := 0
	m.repositories.RetireConnectionInstance = func(_ context.Context, id domain.ConnectionInstanceID) error {
		calls++
		var err error
		checkpoint, err = store.LoadConnectionInstance(id.String())
		if err != nil {
			return err
		}
		if err := store.ArchiveConnectionInstance(id.String()); err != nil {
			return err
		}
		return store.DeleteConnectionInstance(id.String())
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || checkpoint.Lifecycle != "interrupted" || checkpoint.BackendRuntimeID != m.runtimeID {
		t.Fatalf("startup retirement callback: calls=%d, checkpoint=%+v", calls, checkpoint)
	}
	if _, err := store.LoadConnectionInstance(meta.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup left an active connection instance: %v", err)
	}
}

func TestRetirementFallbackKeepsActiveDataOnArchiveFailure(t *testing.T) {
	m, store, meta := retirementFixture(t)
	archiveErr := errors.New("archive unavailable")
	m.repositories.Audit = retirementAuditError{archiveErr}
	if err := m.retirePersisted(t.Context(), meta); !errors.Is(err, archiveErr) {
		t.Fatalf("archive error = %v", err)
	}
	if got, err := store.LoadConnectionInstance(meta.ID); err != nil || got.Lifecycle != "interrupted" {
		t.Fatalf("active metadata lost after failed archive: %+v, %v", got, err)
	}
}

func TestRetirementCallbackFailureDoesNotFallBack(t *testing.T) {
	m, store, meta := retirementFixture(t)
	retireErr := errors.New("transaction failed")
	m.repositories.RetireConnectionInstance = func(context.Context, domain.ConnectionInstanceID) error {
		return retireErr
	}
	if err := m.retirePersisted(t.Context(), meta); !errors.Is(err, retireErr) {
		t.Fatalf("atomic retirement error = %v", err)
	}
	if _, err := store.LoadConnectionInstance(meta.ID); err != nil {
		t.Fatalf("active metadata lost after failed atomic retirement: %v", err)
	}
	if _, err := os.Stat(store.AuditConnectionInstancePath(meta.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("atomic retirement fell back to file archive: %v", err)
	}
}

type retirementWorker struct {
	ports.TerminalWorker
	finalStarted chan struct{}
}

func (w retirementWorker) Snapshot(_ context.Context, _ string, sequence string) ([]byte, string, error) {
	if sequence == "2" {
		close(w.finalStarted)
	}
	return []byte("snapshot " + sequence), sequence, nil
}

func (retirementWorker) CloseSession(context.Context, string) error { return nil }

type blockedPeriodicSnapshot struct {
	ports.TerminalSnapshotRepository
	entered chan struct{}
	release chan struct{}
}

func (r blockedPeriodicSnapshot) SaveSnapshot(ctx context.Context, id domain.ConnectionInstanceID, header domain.SnapshotHeader, payload []byte) error {
	if header.ThroughSequence == "1" {
		close(r.entered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.TerminalSnapshotRepository.SaveSnapshot(ctx, id, header, payload)
}

func waitForRetirement(t *testing.T, ctx context.Context, s *Session) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if s.retiring.Load() {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("retirement did not start")
		}
	}
}

func TestRetirementWaitsForInFlightSnapshotWrite(t *testing.T) {
	m, store, meta := retirementFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	periodic := blockedPeriodicSnapshot{
		TerminalSnapshotRepository: m.repositories.Snapshots, entered: make(chan struct{}), release: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(periodic.release) })
	t.Cleanup(release)
	m.repositories.Snapshots = periodic
	worker := retirementWorker{finalStarted: make(chan struct{})}
	m.worker = worker
	m.repositories.Audit = retirementAuditError{errors.New("unexpected legacy archive")}
	m.repositories.RetireConnectionInstance = func(_ context.Context, id domain.ConnectionInstanceID) error {
		if err := store.ArchiveConnectionInstance(id.String()); err != nil {
			return err
		}
		return store.DeleteConnectionInstance(id.String())
	}
	s := &Session{manager: m, meta: meta, sequence: 1}
	m.sessions[meta.ID] = s
	periodicDone := make(chan error, 1)
	go func() { periodicDone <- s.saveSnapshotWithClosed(ctx, false) }()
	select {
	case <-periodic.entered:
	case <-ctx.Done():
		t.Fatal("periodic snapshot did not reach the repository")
	}
	s.mu.Lock()
	s.closed, s.sequence, s.meta.Lifecycle = true, 2, "exited"
	s.mu.Unlock()

	retirementDone := make(chan error, 1)
	go func() { retirementDone <- m.retireSession(ctx, s) }()
	waitForRetirement(t, ctx, s)
	title := "late title"
	if _, err := m.SetTitle(meta.ID, &title); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("retiring connection instance accepted a title checkpoint: %v", err)
	}
	select {
	case <-worker.finalStarted:
		t.Error("final snapshot overtook the blocked periodic write")
	case <-time.After(25 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal("retirement test timed out")
	}
	release()
	for _, done := range []<-chan error{periodicDone, retirementDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("snapshot or retirement did not finish")
		}
	}
	data, err := os.ReadFile(store.AuditConnectionSnapshotPath(meta.ID))
	if err != nil {
		t.Fatal(err)
	}
	header, payload, err := persistence.DecodeSnapshot(data)
	if err != nil || header.ThroughSequence != "2" || string(payload) != "snapshot 2" {
		t.Fatalf("archived checkpoint = %+v, %q, %v", header, payload, err)
	}
	if _, _, err := store.LoadSnapshot(meta.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired instance has an active snapshot: %v", err)
	}
}

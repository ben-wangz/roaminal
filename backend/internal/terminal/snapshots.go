package terminal

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const snapshotTimeout = 30 * time.Second

func (s *Session) scheduleSnapshotLocked() {
	if s.ephemeral || s.closed || s.retiring.Load() || s.retired.Load() {
		return
	}
	if s.snapshotTimer == nil {
		s.dirtySince = s.manager.now()
		s.snapshotTimer = time.AfterFunc(250*time.Millisecond, s.saveSnapshot)
	}
}

func (s *Session) saveSnapshot() {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()
	_ = s.saveSnapshotWithClosed(ctx, false)
}

func (s *Session) saveSnapshotWithClosed(parent context.Context, final bool) error {
	ctx, cancel := s.manager.storage.withContext(parent)
	defer cancel()
	// The final checkpoint must follow the in-flight timer save, including
	// its repository write. Waiting for it must not extend a caller's deadline.
	if err := s.snapshotMu.lock(ctx); err != nil {
		return err
	}
	defer s.snapshotMu.Unlock()
	if err := s.mu.lock(ctx); err != nil {
		return err
	}
	if s.ephemeral || s.retired.Load() || (!final && (s.closed || s.retiring.Load())) {
		s.mu.Unlock()
		return nil
	}
	if final && s.snapshotTimer != nil {
		s.snapshotTimer.Stop()
	}
	sequence, meta := s.sequence, s.meta
	s.snapshotTimer, s.dirtySince = nil, time.Time{}
	s.mu.Unlock()

	if err := s.checkpoint(ctx, meta, sequence); err != nil {
		if ctx.Err() == nil {
			s.manager.storeDegraded(ctx, meta.ID, err)
		}
		return err
	}
	if !final && s.mu.lock(ctx) == nil {
		if s.sequence != sequence {
			s.scheduleSnapshotLocked()
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *Session) checkpoint(ctx context.Context, meta domain.ConnectionInstanceMeta, sequence uint64) error {
	m := s.manager
	if m.worker == nil {
		return fmt.Errorf("terminal worker unavailable")
	}
	payload, through, err := m.worker.Snapshot(ctx, meta.ID, strconv.FormatUint(sequence, 10))
	if err != nil || !m.hasPersistence() {
		return err
	}
	header := domain.SnapshotHeader{Cols: meta.Cols, Rows: meta.Rows, ScrollbackLines: m.cfg.ScrollbackLines, ThroughSequence: through}
	return m.storage.run(ctx, func(ctx context.Context) error {
		return m.repositories.saveSnapshot(ctx, domain.ConnectionInstanceID(meta.ID), header, payload)
	})
}

func (s *Session) broadcastMessageLocked(message streamMessage) {
	s.streamSequence++
	data := message(streamEnvelope(s.streamSequence, s.manager.now().UTC(), s.manager.ids))
	for client := range s.clients {
		if !client.enqueue(data, true) {
			delete(s.clients, client)
		}
	}
}

func (m *Manager) storeDegraded(ctx context.Context, id string, cause error) {
	if m.repositories.Instances != nil {
		_ = m.storage.run(ctx, func(ctx context.Context) error {
			return m.repositories.Instances.MarkConnectionInstanceDegraded(ctx, domain.ConnectionInstanceID(id))
		})
	}
	fmt.Fprintf(os.Stderr, "Roaminal persistence degraded: %v\n", cause)
}

func (m *Manager) fail(err error) {
	select {
	case m.fatal <- err:
	default:
	}
}

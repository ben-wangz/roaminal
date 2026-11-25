package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

const retirementTimeout = 30 * time.Second

// retireSession saves the final PTY checkpoint before retiring the connection
// instance. On failure, leave active data for retry or startup recovery.
func (m *Manager) retireSession(parent context.Context, session *Session) error {
	ctx, cancel := m.storage.withContext(parent)
	defer cancel()
	// Join an existing retirement rather than reporting success before its
	// archive commits. A failed attempt can be retried while budget remains.
	if err := session.retireMu.lock(ctx); err != nil {
		return err
	}
	defer session.retireMu.Unlock()
	if session.retired.Load() {
		return nil
	}
	if err := session.awaitExitHook(ctx); err != nil {
		return err
	}
	session.retiring.Store(true)
	defer session.retiring.Store(false)

	if err := session.waitReadLoop(ctx); err != nil {
		return fmt.Errorf("drain PTY: %w", err)
	}
	if m.hasPersistence() {
		if err := session.saveSnapshotWithClosed(ctx, true); err != nil {
			return fmt.Errorf("save final snapshot: %w", err)
		}
	}
	if err := session.mu.lock(ctx); err != nil {
		return err
	}
	session.closeClientsLocked()
	session.meta.UpdatedAt = m.now().UTC()
	meta := session.meta
	session.mu.Unlock()
	if m.hasPersistence() {
		if err := m.saveMetaContext(ctx, meta); err != nil {
			return fmt.Errorf("save final metadata: %w", err)
		}
		if err := m.retirePersistedInstance(ctx, meta.ID); err != nil {
			return err
		}
	}

	m.removeRetired(session, meta.ID)
	if m.worker != nil {
		if err := m.worker.CloseSession(ctx, meta.ID); err != nil && !errors.Is(err, os.ErrNotExist) && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "Roaminal worker session %s cleanup warning: %v\n", meta.ID, err)
		}
	}
	return nil
}

func (m *Manager) removeRetired(session *Session, id string) {
	session.retired.Store(true)
	m.mu.Lock()
	if m.sessions[id] == session {
		delete(m.sessions, id)
	}
	if m.pending[id] == session {
		delete(m.pending, id)
	}
	m.mu.Unlock()
}

func (s *Session) closeClientsLocked() {
	for client := range s.clients {
		client.close()
	}
	clear(s.clients)
	s.controlOwner = nil
}

func (s *Session) closePTY() {
	if s.pty != nil {
		_ = s.pty.Close()
	}
}

func (s *Session) waitReadLoop(ctx context.Context) error {
	if s.readDone == nil {
		return ctx.Err()
	}
	select {
	case <-s.readDone:
		return ctx.Err()
	case <-ctx.Done():
		s.closePTY()
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
	// A process can exit while a child still holds the PTY open.
	s.closePTY()
	select {
	case <-s.readDone:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		return errors.New("PTY reader did not stop")
	}
}

func (m *Manager) terminateSession(parent context.Context, session *Session, lifecycle string) error {
	ctx, stop := context.WithTimeout(parent, retirementTimeout)
	defer stop()
	ctx, cancel := m.storage.withContext(ctx)
	defer cancel()
	defer session.closePTY()
	if err := session.mu.lock(ctx); err != nil {
		// PTY and process handles are immutable after publication. A stalled
		// input, attach or checkpoint must not prevent signalling the process.
		_ = terminateSessionProcessGroup(ctx, session.cmd)
		return err
	}
	if session.retired.Load() {
		session.mu.Unlock()
		return os.ErrNotExist
	}
	if !session.closed {
		session.closed = true
		session.meta.Lifecycle = lifecycle
		if lifecycle == "interrupted" {
			session.meta.BackendRuntimeID = m.runtimeID
		}
		session.meta.UpdatedAt = m.now().UTC()
	}
	if session.snapshotTimer != nil {
		session.snapshotTimer.Stop()
		session.snapshotTimer = nil
	}
	session.closeClientsLocked()
	hook := session.takeExitHookLocked()
	ephemeral := session.ephemeral
	session.mu.Unlock()

	terminateErr := terminateSessionProcessGroup(ctx, session.cmd)
	if hook != nil {
		// Explicit termination is not a successful process exit. The hook still
		// releases resources owned by the process.
		hook(ctx, ExitStatus{})
	}
	if ephemeral {
		m.finishEphemeral(ctx, session)
		return terminateErr
	}
	retireErr := m.retireSession(ctx, session)
	if terminateErr != nil {
		return terminateErr
	}
	return retireErr
}

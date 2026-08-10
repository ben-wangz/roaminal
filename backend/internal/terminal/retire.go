package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// retireSession is the only path that removes a connection instance from the
// active store. The audit copy is committed before the active directory is
// deleted, so a persistence failure leaves data recoverable.
func (m *Manager) retireSession(ctx context.Context, session *Session) error {
	session.mu.Lock()
	if session.retired {
		session.mu.Unlock()
		return nil
	}
	if session.retiring {
		session.mu.Unlock()
		return nil
	}
	session.retiring = true
	session.mu.Unlock()

	defer func() {
		session.mu.Lock()
		session.retiring = false
		session.mu.Unlock()
	}()

	session.waitReadLoop()
	if m.store != nil {
		if err := session.saveSnapshotFinal(); err != nil {
			return fmt.Errorf("save final snapshot: %w", err)
		}
		session.mu.Lock()
		session.meta.UpdatedAt = time.Now().UTC()
		meta := session.meta
		session.mu.Unlock()
		if err := m.store.SaveSession(meta); err != nil {
			return fmt.Errorf("save final metadata: %w", err)
		}
		if err := m.store.ArchiveSession(meta.ID); err != nil {
			return fmt.Errorf("archive session: %w", err)
		}
		if err := m.store.DeleteSession(meta.ID); err != nil {
			return fmt.Errorf("remove active session: %w", err)
		}
	}

	session.mu.Lock()
	for client := range session.clients {
		client.close()
	}
	session.clients = make(map[*Client]struct{})
	session.controlOwner = nil
	session.retired = true
	id := session.meta.ID
	session.mu.Unlock()

	m.mu.Lock()
	if current := m.sessions[id]; current == session {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if m.worker != nil {
		if err := m.worker.CloseSession(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "Roaminal worker session %s cleanup warning: %v\n", id, err)
		}
	}
	return nil
}

func (s *Session) waitReadLoop() {
	if s.readDone == nil {
		return
	}
	select {
	case <-s.readDone:
		return
	case <-time.After(2 * time.Second):
	}
	// A process can exit while the PTY reader remains blocked. Closing the
	// descriptor releases it without waiting indefinitely during cleanup.
	s.mu.Lock()
	if s.pty != nil {
		_ = s.pty.Close()
	}
	s.mu.Unlock()
	select {
	case <-s.readDone:
	case <-time.After(500 * time.Millisecond):
	}
}

func (m *Manager) terminateSession(ctx context.Context, session *Session, lifecycle string) error {
	session.mu.Lock()
	if session.retired {
		session.mu.Unlock()
		return os.ErrNotExist
	}
	cmd := session.cmd
	session.closed = true
	session.meta.Lifecycle = lifecycle
	session.meta.UpdatedAt = time.Now().UTC()
	if session.pty != nil {
		_ = session.pty.Close()
	}
	for client := range session.clients {
		client.close()
	}
	session.clients = make(map[*Client]struct{})
	session.controlOwner = nil
	session.mu.Unlock()
	terminateErr := terminateSessionProcessGroup(ctx, cmd)
	if onExit := session.takeExitHook(); onExit != nil {
		// Explicit termination is not a successful process exit. The hook still
		// must run so resources owned by the process can be released.
		onExit(ExitStatus{})
	}
	if session.ephemeral {
		m.finishEphemeral(ctx, session)
		return terminateErr
	}
	retireErr := m.retireSession(ctx, session)
	if terminateErr != nil {
		return terminateErr
	}
	return retireErr
}

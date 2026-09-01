package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// CreateProcess starts a managed PTY for a fixed argv. It is used by the SSH
// connection manager; arbitrary command templates are never exposed to HTTP.
func (m *Manager) CreateProcess(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string) (Summary, error) {
	return m.createProcess(ctx, meta, argv, extraEnv, false, "", nil, nil)
}

// CreateProcessWithExit starts a fixed process and invokes onExit after its
// PTY exits, outside the session lock. The callback receives no input/output.
func (m *Manager) CreateProcessWithExit(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, onExit func(ExitStatus)) (Summary, error) {
	return m.createProcess(ctx, meta, argv, extraEnv, false, "", nil, onExit)
}

// CreatePendingProcess starts a runtime-only launch. It is attachable over the
// launch websocket, but is excluded from persistence, heartbeat summaries, and
// audit until PromotePending is called.
func (m *Manager) CreatePendingProcess(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, onMarker func(string), onExit func(ExitStatus)) (Summary, error) {
	return m.CreatePendingProcessOwned(ctx, meta, argv, extraEnv, "", onMarker, onExit)
}

func (m *Manager) CreatePendingProcessOwned(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, ownerID string, onMarker func(string), onExit func(ExitStatus)) (Summary, error) {
	return m.createProcess(ctx, meta, argv, extraEnv, true, ownerID, onMarker, onExit)
}

func (m *Manager) createProcess(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, ephemeral bool, ownerID string, onMarker func(string), onExit func(ExitStatus)) (Summary, error) {
	if len(argv) == 0 {
		return Summary{}, errors.New("empty process argv")
	}
	if meta.Cols == 0 {
		meta.Cols = 120
	}
	if meta.Rows == 0 {
		meta.Rows = 30
	}
	if meta.Cols < 2 || meta.Cols > 1000 || meta.Rows < 1 || meta.Rows > 1000 {
		return Summary{}, errors.New("invalid terminal dimensions")
	}
	cwd := meta.Cwd
	if cwd == "" {
		cwd = m.cfg.InitialCwd
	}
	if !filepath.IsAbs(cwd) {
		return Summary{}, errors.New("cwd must be absolute")
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return Summary{}, errors.New("cwd is not accessible")
	}
	if meta.InitialCwd == "" {
		meta.InitialCwd = cwd
	}
	if meta.Cwd == "" {
		meta.Cwd = cwd
	}
	if meta.ID == "" {
		id, err := m.newID()
		if err != nil {
			return Summary{}, err
		}
		meta.ID = id
	}
	now := m.now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	if meta.BackendRuntimeID == "" {
		meta.BackendRuntimeID = m.runtimeID
	}
	if meta.Lifecycle == "" {
		meta.Lifecycle = "live"
	}
	if ephemeral {
		meta.Lifecycle = "pending"
	}
	if meta.SourceState == "" {
		meta.SourceState = "current"
	}
	meta.TerminalType = effectiveTerminalType(meta.TerminalType)
	m.mu.Lock()
	if len(m.sessions)+len(m.pending)+m.createReservations >= m.connectionLimit() {
		m.mu.Unlock()
		return Summary{}, ErrConnectionCapacity
	}
	m.createReservations++
	m.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			m.mu.Lock()
			m.createReservations--
			m.mu.Unlock()
		}
	}()
	if err := m.worker.Create(ctx, meta.ID, meta.Cols, meta.Rows, m.cfg.ScrollbackLines); err != nil {
		return Summary{}, err
	}
	session, err := m.startCommand(meta, cwd, argv, extraEnv)
	if err != nil {
		_ = m.worker.CloseSession(ctx, meta.ID)
		return Summary{}, err
	}
	session.onExit = onExit
	session.ephemeral = ephemeral
	session.onMarker = onMarker
	session.pendingOwner = ownerID
	if ephemeral {
		session.lastActivity = m.now()
		session.detachedAt = session.lastActivity
	}
	if m.hasPersistence() && !ephemeral {
		if err := m.saveMeta(meta); err != nil {
			m.abortSession(ctx, session, true)
			return Summary{}, err
		}
	}
	m.mu.Lock()
	m.createReservations--
	reserved = false
	if ephemeral {
		m.pending[meta.ID] = session
	} else {
		m.sessions[meta.ID] = session
	}
	m.mu.Unlock()
	m.startLoops(session)
	if ephemeral {
		go m.watchPending(session)
	}
	return m.summary(session), nil
}

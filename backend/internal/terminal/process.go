package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

// CreateProcess starts a managed PTY for a fixed argv. It is used by the SSH
// connection manager; arbitrary command templates are never exposed to HTTP.
func (m *Manager) CreateProcess(ctx context.Context, meta persistence.SessionMeta, argv []string, extraEnv []string) (Summary, error) {
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
	if meta.ID == "" {
		id, err := newID()
		if err != nil {
			return Summary{}, err
		}
		meta.ID = id
	}
	now := time.Now().UTC()
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
	if meta.SourceState == "" {
		meta.SourceState = "current"
	}
	m.mu.Lock()
	if len(m.sessions)+m.createReservations >= m.connectionLimit() {
		m.mu.Unlock()
		return Summary{}, errors.New("connection capacity reached")
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
	m.startLoops(session)
	if m.store != nil {
		if err := m.store.SaveSession(meta); err != nil {
			m.abortSession(ctx, session, true)
			return Summary{}, err
		}
	}
	m.mu.Lock()
	m.createReservations--
	reserved = false
	m.sessions[meta.ID] = session
	m.mu.Unlock()
	return m.summary(session), nil
}

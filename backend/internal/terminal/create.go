package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func (m *Manager) Create(ctx context.Context, cwd string, cols, rows int) (Summary, error) {
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 30
	}
	if cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		return Summary{}, errors.New("invalid terminal dimensions")
	}
	ctx, finish, err := m.beginCreate(ctx)
	if err != nil {
		return Summary{}, err
	}
	published := false
	defer func() { finish(published) }()
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	type activity struct {
		cwd                  string
		updatedAt, createdAt time.Time
		id                   string
	}
	activities := make([]activity, 0, len(sessions))
	for _, session := range sessions {
		if err := session.mu.lock(ctx); err != nil {
			return Summary{}, err
		}
		if !session.closed {
			activities = append(activities, activity{cwd: session.meta.Cwd, updatedAt: session.meta.UpdatedAt, createdAt: session.meta.CreatedAt, id: session.meta.ID})
		}
		session.mu.Unlock()
	}
	sort.Slice(activities, func(i, j int) bool {
		if activities[i].updatedAt.Equal(activities[j].updatedAt) {
			if activities[i].createdAt.Equal(activities[j].createdAt) {
				return activities[i].id > activities[j].id
			}
			return activities[i].createdAt.After(activities[j].createdAt)
		}
		return activities[i].updatedAt.After(activities[j].updatedAt)
	})
	inherited := ""
	if len(activities) > 0 {
		inherited = activities[0].cwd
	}
	if cwd == "" {
		cwd = inherited
	}
	if cwd == "" {
		cwd = m.cfg.InitialCwd
	}
	if !filepath.IsAbs(cwd) {
		return Summary{}, errors.New("cwd must be absolute")
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return Summary{}, errors.New("cwd is not accessible")
	}
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	now := m.now().UTC()
	meta := domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: m.runtimeID, ConnectionDefinitionID: "local", Type: "local", Purpose: "interactive", Lifecycle: "live", SourceState: "current", InitialCwd: cwd, Cwd: cwd, Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now}
	meta.TerminalType = effectiveTerminalType(meta.TerminalType)
	session, err := m.startSession(ctx, meta, cwd, true)
	if err != nil {
		return Summary{}, err
	}
	if m.hasPersistence() {
		if err := m.saveMetaContext(ctx, meta); err != nil {
			m.abortSession(ctx, session)
			return Summary{}, err
		}
	}
	result, err := m.publishSession(ctx, session)
	if err != nil {
		m.abortSession(ctx, session)
		return Summary{}, err
	}
	published = true
	return result, nil
}

func (m *Manager) beginCreate(parent context.Context) (context.Context, func(bool), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return nil, nil, os.ErrProcessDone
	}
	if len(m.sessions)+len(m.pending)+m.createReservations >= m.connectionLimit() {
		return nil, nil, ErrConnectionCapacity
	}
	m.createReservations++
	m.creates.Add(1)
	ctx, cancel := cancelWith(parent, m.createCtx)
	return ctx, func(published bool) {
		cancel()
		if !published {
			m.mu.Lock()
			m.createReservations--
			m.mu.Unlock()
		}
		m.creates.Done()
	}, nil
}

// Transfer ownership of the process from creation to the manager's shutdown list.
func (m *Manager) publishSession(ctx context.Context, session *Session) (Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return Summary{}, os.ErrProcessDone
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}
	m.createReservations--
	id := session.meta.ID
	result := m.summaryLocked(session) // The session is not yet shared.
	if session.ephemeral {
		m.pending[id] = session
		go m.watchPending(session)
	} else {
		m.sessions[id] = session
	}
	m.startLoops(session)
	return result, nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	return m.terminateSession(ctx, session, "exited")
}

func (m *Manager) connectionLimit() int {
	return m.cfg.MaxConnectionInstances
}

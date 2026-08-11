package terminal

import (
	"context"
	"errors"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"os"
	"path/filepath"
	"sort"
	"time"
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
	m.mu.Lock()
	if len(m.sessions)+len(m.pending)+m.createReservations >= m.connectionLimit() {
		m.mu.Unlock()
		return Summary{}, ErrConnectionCapacity
	}
	m.createReservations++
	type activity struct {
		cwd                  string
		updatedAt, createdAt time.Time
		id                   string
	}
	activities := make([]activity, 0, len(m.sessions))
	for _, session := range m.sessions {
		session.mu.Lock()
		activities = append(activities, activity{cwd: session.meta.Cwd, updatedAt: session.meta.UpdatedAt, createdAt: session.meta.CreatedAt, id: session.meta.ID})
		session.mu.Unlock()
	}
	m.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			m.mu.Lock()
			m.createReservations--
			m.mu.Unlock()
		}
	}()
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
	id, err := newID()
	if err != nil {
		return Summary{}, err
	}
	now := time.Now().UTC()
	meta := persistence.ConnectionInstanceMeta{FormatVersion: persistence.ConnectionFormatVersion, ID: id, BackendRuntimeID: m.runtimeID, ConnectionDefinitionID: "local", Type: "local", Purpose: "interactive", Lifecycle: "live", SourceState: "current", InitialCwd: cwd, Cwd: cwd, Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now}
	session, err := m.startSession(ctx, meta, cwd, true)
	if err != nil {
		return Summary{}, err
	}
	if m.store != nil {
		if err := m.store.SaveConnectionInstance(meta); err != nil {
			m.abortSession(ctx, session, true)
			return Summary{}, err
		}
	}
	m.mu.Lock()
	m.createReservations--
	reserved = false
	m.sessions[id] = session
	m.mu.Unlock()
	m.startLoops(session)
	return m.summary(session), nil
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

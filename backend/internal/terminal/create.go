package terminal

import (
	"context"
	"errors"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"os"
	"path/filepath"
	"sort"
	"syscall"
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
	if len(m.sessions)+m.createReservations >= m.cfg.MaxSessions {
		m.mu.Unlock()
		return Summary{}, errors.New("session capacity reached")
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
	meta := persistence.SessionMeta{FormatVersion: persistence.SessionFormatVersion, ID: id, InitialCwd: cwd, Cwd: cwd, Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now, Executions: []persistence.ExecutionRecord{}}
	session, err := m.startSession(ctx, meta, cwd, true)
	if err != nil {
		return Summary{}, err
	}
	if err := m.store.SaveSession(meta); err != nil {
		m.abortSession(ctx, session, true)
		return Summary{}, err
	}
	m.mu.Lock()
	m.createReservations--
	reserved = false
	m.sessions[id] = session
	m.mu.Unlock()
	return m.summary(session), nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return os.ErrNotExist
	}
	session.mu.Lock()
	if !session.closed {
		session.closed = true
		_ = signalProcessGroup(session.cmd, syscall.SIGTERM)
		_ = session.pty.Close()
		_ = m.worker.CloseSession(ctx, id)
	}
	for client := range session.clients {
		client.close()
	}
	session.clients = make(map[*Client]struct{})
	session.controlOwner = nil
	session.mu.Unlock()
	return m.store.DeleteSession(id)
}

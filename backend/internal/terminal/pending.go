package terminal

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const (
	pendingReconnectGrace = 15 * time.Second
	pendingIdleTimeout    = 5 * time.Minute
)

func (m *Manager) pendingSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pending[id]
}

func (m *Manager) pendingOrSession(id string) *Session {
	m.mu.RLock()
	session := m.pending[id]
	if session == nil {
		session = m.sessions[id]
	}
	m.mu.RUnlock()
	return session
}

func (m *Manager) TouchPending(id string) {
	session := m.pendingSession(id)
	if session == nil {
		return
	}
	session.mu.Lock()
	if session.ephemeral {
		session.lastActivity = m.now()
		session.detachedAt = time.Time{}
	}
	session.mu.Unlock()
}

func (m *Manager) PendingOwner(id string) string {
	session := m.pendingOrSession(id)
	if session == nil {
		return ""
	}
	session.mu.Lock()
	owner := session.pendingOwner
	session.mu.Unlock()
	return owner
}

func (m *Manager) watchPending(session *Session) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		session.mu.Lock()
		if !session.ephemeral || session.closed {
			session.mu.Unlock()
			return
		}
		now := m.now()
		lastActivity := session.lastActivity
		detachedAt := session.detachedAt
		id := session.meta.ID
		noClients := len(session.clients) == 0
		session.mu.Unlock()
		if now.Sub(lastActivity) >= pendingIdleTimeout || (noClients && !detachedAt.IsZero() && now.Sub(detachedAt) >= pendingReconnectGrace) {
			_ = m.AbortPending(m.createCtx, id)
			return
		}
	}
}

// PromotePending publishes a successful runtime launch as a normal connection
// instance. The final metadata is persisted before the instance enters the
// heartbeat-visible map.
func (m *Manager) PromotePending(id string, meta domain.ConnectionInstanceMeta) (Summary, error) {
	session := m.pendingSession(id)
	if session == nil {
		return Summary{}, os.ErrNotExist
	}
	ctx, cancel := context.WithTimeout(m.createCtx, 5*time.Second)
	defer cancel()
	if err := session.mu.lock(ctx); err != nil {
		return Summary{}, err
	}
	defer session.mu.Unlock()
	if session.closed || !session.ephemeral {
		return Summary{}, errors.New("pending launch is no longer active")
	}
	meta.ID = id
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = session.meta.CreatedAt
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = m.now().UTC()
	}
	if meta.BackendRuntimeID == "" {
		meta.BackendRuntimeID = m.runtimeID
	}
	if meta.InitialCwd == "" {
		meta.InitialCwd = session.meta.InitialCwd
	}
	if meta.Cwd == "" {
		meta.Cwd = session.meta.Cwd
	}
	if meta.Cols == 0 {
		meta.Cols = session.meta.Cols
	}
	if meta.Rows == 0 {
		meta.Rows = session.meta.Rows
	}
	if meta.TerminalType == "" {
		meta.TerminalType = session.meta.TerminalType
	}
	meta.TerminalType = effectiveTerminalType(meta.TerminalType)
	meta.Lifecycle = "live"
	meta.SourceState = "current"
	if m.hasPersistence() {
		if err := m.saveMetaContext(ctx, meta); err != nil {
			return Summary{}, err
		}
	}
	// Keep the session locked across the transition so shutdown or process exit
	// cannot observe a persisted launch that's still treated as ephemeral.
	session.meta = meta
	session.ephemeral = false
	session.published = true
	session.onMarker = nil
	m.mu.Lock()
	delete(m.pending, id)
	m.sessions[id] = session
	m.mu.Unlock()
	result := m.summaryLocked(session)
	session.broadcastMessageLocked(launchPublishedStreamMessage(result))
	return result, nil
}

// AbortPending stops a launch without creating a session or audit artifact.
func (m *Manager) AbortPending(ctx context.Context, id string) error {
	session := m.pendingSession(id)
	if session == nil {
		return os.ErrNotExist
	}
	return m.terminateSession(ctx, session, "cancelled")
}

func (m *Manager) finishEphemeral(ctx context.Context, session *Session) {
	if session.awaitExitHook(ctx) != nil || session.mu.lock(ctx) != nil {
		return
	}
	if session.retired.Load() {
		session.mu.Unlock()
		return
	}
	session.closeClientsLocked()
	session.retired.Store(true)
	id := session.meta.ID
	session.mu.Unlock()
	m.removeRetired(session, id)
	if m.worker != nil {
		_ = m.worker.CloseSession(ctx, id)
	}
}

package terminal

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"
)

func (m *Manager) ReserveAttach(id string) error {
	return m.reserveAttach(id, false)
}
func (m *Manager) ReservePendingAttach(id string) error {
	return m.reserveAttach(id, true)
}
func (m *Manager) reserveAttach(id string, pending bool) error {
	var session *Session
	if pending {
		session = m.pendingOrSession(id)
	} else {
		m.mu.RLock()
		session = m.sessions[id]
		m.mu.RUnlock()
	}
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if pending && session.ephemeral {
		session.lastActivity = time.Now()
		session.detachedAt = time.Time{}
	}
	if len(session.clients)+session.reservations >= m.clientLimit() {
		return ErrClientCapacity
	}
	session.reservations++
	return nil
}
func (m *Manager) ReleaseAttach(id string) {
	m.ReleasePendingAttach(id)
}
func (m *Manager) ReleasePendingAttach(id string) {
	session := m.pendingOrSession(id)
	if session == nil {
		return
	}
	session.mu.Lock()
	if session.reservations > 0 {
		session.reservations--
	}
	session.mu.Unlock()
}
func (m *Manager) Attach(ctx context.Context, id string) (*Client, error) {
	return m.attach(ctx, id, false)
}
func (m *Manager) AttachReserved(ctx context.Context, id string) (*Client, error) {
	return m.attach(ctx, id, true)
}
func (m *Manager) AttachPendingReserved(ctx context.Context, id string) (*Client, error) {
	return m.attachPending(ctx, id, true)
}
func (m *Manager) attach(ctx context.Context, id string, reserved bool) (*Client, error) {
	return m.attachPendingMode(ctx, id, reserved, false)
}
func (m *Manager) attachPending(ctx context.Context, id string, reserved bool) (*Client, error) {
	return m.attachPendingMode(ctx, id, reserved, true)
}
func (m *Manager) attachPendingMode(ctx context.Context, id string, reserved, pending bool) (*Client, error) {
	var session *Session
	if pending {
		session = m.pendingOrSession(id)
	} else {
		m.mu.RLock()
		session = m.sessions[id]
		m.mu.RUnlock()
	}
	if session == nil {
		return nil, os.ErrNotExist
	}
	client := newClient()
	session.mu.Lock()
	defer session.mu.Unlock()
	closed := session.closed
	if reserved {
		if session.reservations < 1 {
			return nil, ErrClientCapacity
		}
		session.reservations--
	} else if len(session.clients)+session.reservations >= m.clientLimit() {
		return nil, ErrClientCapacity
	}
	sequence := strconv.FormatUint(session.sequence, 10)
	snapshot, _, err := m.worker.Snapshot(ctx, id, sequence)
	if err != nil {
		if m.store != nil {
			if _, payload, loadErr := m.store.LoadSnapshot(id); loadErr == nil {
				snapshot = payload
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	client.enqueue(message(map[string]any{"type": "snapshot", "data": string(snapshot)}), false)
	client.enqueue(message(map[string]any{"type": "meta", "title": session.meta.EffectiveTitle(), "titleMode": titleMode(session.meta), "cwd": session.meta.Cwd, "cols": session.meta.Cols, "rows": session.meta.Rows, "attention": session.attention, "sourceState": session.meta.SourceState, "generationStatus": session.meta.GenerationStatus, "generationError": session.meta.GenerationError, "generationStaging": session.meta.GenerationStaging}), false)
	if closed {
		client.enqueue(message(map[string]any{"type": "status", "status": "terminated", "exitStatus": session.exitStatus}), false)
	} else {
		client.enqueue(message(map[string]any{"type": "status", "status": "ready"}), false)
	}
	if pending && session.published {
		client.enqueue(message(map[string]any{"type": "launch_published", "instance": m.summaryLocked(session)}), false)
	}
	session.clients[client] = struct{}{}
	return client, nil
}

func (m *Manager) clientLimit() int {
	if m.cfg.MaxClientsPerConnectionInstance > 0 {
		return m.cfg.MaxClientsPerConnectionInstance
	}
	return m.cfg.MaxClientsPerSession
}
func (m *Manager) Detach(id string, client *Client) {
	m.detach(id, client, false)
}
func (m *Manager) DetachPending(id string, client *Client) {
	m.detach(id, client, true)
}
func (m *Manager) detach(id string, client *Client, pending bool) {
	var session *Session
	if pending {
		session = m.pendingOrSession(id)
	} else {
		m.mu.RLock()
		session = m.sessions[id]
		m.mu.RUnlock()
	}
	if session == nil {
		client.close()
		return
	}
	session.mu.Lock()
	delete(session.clients, client)
	if session.controlOwner == client {
		session.controlOwner = nil
	}
	client.close()
	if pending && session.ephemeral && len(session.clients) == 0 {
		session.detachedAt = time.Now()
	}
	session.mu.Unlock()
}
func (m *Manager) ClaimControl(id string, client *Client) error {
	return m.claimControl(id, client, false)
}
func (m *Manager) ClaimPendingControl(id string, client *Client) error {
	return m.claimControl(id, client, true)
}
func (m *Manager) claimControl(id string, client *Client, pending bool) error {
	var session *Session
	if pending {
		session = m.pendingOrSession(id)
	} else {
		m.mu.RLock()
		session = m.sessions[id]
		m.mu.RUnlock()
	}
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return os.ErrProcessDone
	}
	if _, ok := session.clients[client]; !ok {
		return errors.New("client is not attached")
	}
	session.controlOwner = client
	session.attention = false
	return nil
}

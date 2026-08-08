package terminal

import (
	"context"
	"errors"
	"os"
	"strconv"
)

func (m *Manager) ReserveAttach(id string) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
	if session == nil {
		return os.ErrNotExist
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.clients)+session.reservations >= m.clientLimit() {
		return ErrClientCapacity
	}
	session.reservations++
	return nil
}
func (m *Manager) ReleaseAttach(id string) {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
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
func (m *Manager) attach(ctx context.Context, id string, reserved bool) (*Client, error) {
	m.mu.RLock()
	session, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
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
		if _, payload, loadErr := m.store.LoadSnapshot(id); loadErr == nil {
			snapshot = payload
		} else {
			return nil, err
		}
	}
	client.enqueue(message(map[string]any{"type": "snapshot", "data": string(snapshot)}), false)
	client.enqueue(message(map[string]any{"type": "meta", "title": session.meta.EffectiveTitle(), "titleMode": titleMode(session.meta), "cwd": session.meta.Cwd, "cols": session.meta.Cols, "rows": session.meta.Rows, "attention": session.attention}), false)
	if closed {
		client.enqueue(message(map[string]any{"type": "status", "status": "terminated", "exitStatus": session.exitStatus}), false)
	} else {
		client.enqueue(message(map[string]any{"type": "status", "status": "ready"}), false)
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
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
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
	session.mu.Unlock()
}
func (m *Manager) ClaimControl(id string, client *Client) error {
	m.mu.RLock()
	session := m.sessions[id]
	m.mu.RUnlock()
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

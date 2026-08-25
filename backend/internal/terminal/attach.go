package terminal

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
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
		session.lastActivity = m.now()
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
func (m *Manager) Attach(ctx context.Context, id string) (ports.TerminalClient, error) {
	return m.attach(ctx, id, false)
}
func (m *Manager) AttachReserved(ctx context.Context, id string) (ports.TerminalClient, error) {
	return m.attach(ctx, id, true)
}
func (m *Manager) AttachPendingReserved(ctx context.Context, id string) (ports.TerminalClient, error) {
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
		if m.hasPersistence() {
			if payload, loadErr := m.loadSnapshot(ctx, id); loadErr == nil {
				snapshot = payload
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	client.enqueue(session.stampInitialLocked(snapshotStreamMessage(string(snapshot))), false)
	client.enqueue(session.stampInitialLocked(metaStreamMessage(MetaMessage{Title: session.meta.EffectiveTitle(), TitleMode: titleMode(session.meta), Cwd: session.meta.Cwd, Cols: session.meta.Cols, Rows: session.meta.Rows, Attention: session.attention, SourceState: session.meta.SourceState, GenerationStatus: session.meta.GenerationStatus, GenerationError: session.meta.GenerationError})), false)
	if closed {
		client.enqueue(session.stampInitialLocked(terminatedStreamMessage(session.exitStatus)), false)
	} else {
		client.enqueue(session.stampInitialLocked(readyStreamMessage()), false)
	}
	if pending && session.published {
		client.enqueue(session.stampInitialLocked(launchPublishedStreamMessage(m.summaryLocked(session))), false)
	}
	session.clients[client] = struct{}{}
	return client, nil
}

func (s *Session) stampInitialLocked(message streamMessage) []byte {
	s.streamSequence++
	return message(streamEnvelope(s.streamSequence, s.manager.now().UTC(), s.manager.ids))
}

func (m *Manager) clientLimit() int {
	return m.cfg.MaxClientsPerConnectionInstance
}
func (m *Manager) Detach(id string, client ports.TerminalClient) {
	concrete, err := concreteClient(client)
	if err != nil {
		return
	}
	m.detach(id, concrete, false)
}
func (m *Manager) DetachPending(id string, client ports.TerminalClient) {
	concrete, err := concreteClient(client)
	if err != nil {
		return
	}
	m.detach(id, concrete, true)
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
		session.detachedAt = m.now()
	}
	session.mu.Unlock()
}
func (m *Manager) ClaimControl(id string, client ports.TerminalClient) error {
	concrete, err := concreteClient(client)
	if err != nil {
		return err
	}
	return m.claimControl(id, concrete, false)
}
func (m *Manager) ClaimPendingControl(id string, client ports.TerminalClient) error {
	concrete, err := concreteClient(client)
	if err != nil {
		return err
	}
	return m.claimControl(id, concrete, true)
}

func concreteClient(client ports.TerminalClient) (*Client, error) {
	concrete, ok := client.(*Client)
	if !ok || concrete == nil {
		return nil, errors.New("invalid terminal client")
	}
	return concrete, nil
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

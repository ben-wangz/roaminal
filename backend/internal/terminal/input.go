package terminal

import (
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/creack/pty"
	"os"
	"strconv"
	"time"
)

func (m *Manager) Input(id string, client ports.TerminalClient, data string) error {
	concrete, err := concreteClient(client)
	if err != nil {
		return err
	}
	return m.input(id, concrete, data, false)
}
func (m *Manager) InputPending(id string, client ports.TerminalClient, data string) error {
	concrete, err := concreteClient(client)
	if err != nil {
		return err
	}
	return m.input(id, concrete, data, true)
}
func (m *Manager) input(id string, client *Client, data string, pending bool) error {
	if len([]byte(data)) > 1024*1024 {
		return errors.New("input too large")
	}
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
	select {
	case <-client.Done():
		return errors.New("client closed")
	default:
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.ephemeral {
		session.lastActivity = m.now()
		session.detachedAt = time.Time{}
	}
	if session.closed {
		return os.ErrProcessDone
	}
	if session.controlOwner != client {
		return ErrControlNotOwner
	}
	if session.currentExec != nil {
		combined := append([]byte(session.currentExec.Command), []byte(data)...)
		if len(combined) > 64*1024 {
			combined = combined[:64*1024]
			session.currentExec.Truncated = true
		}
		commandBytes := []byte(session.currentExec.Command)
		if len(commandBytes) > len(combined) {
			commandBytes = combined
		}
		session.currentExec.Input = string(combined[len(commandBytes):])
	}
	_, err := session.pty.Write([]byte(data))
	return err
}

func (m *Manager) Resize(id string, client ports.TerminalClient, cols, rows int) error {
	concrete, err := concreteClient(client)
	if err != nil {
		return err
	}
	return m.resize(id, concrete, cols, rows, false)
}
func (m *Manager) ResizePending(id string, client ports.TerminalClient, cols, rows int) error {
	concrete, err := concreteClient(client)
	if err != nil {
		return err
	}
	return m.resize(id, concrete, cols, rows, true)
}
func (m *Manager) resize(id string, client *Client, cols, rows int, pending bool) error {
	if cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		return errors.New("invalid terminal dimensions")
	}
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
	if session.ephemeral {
		session.lastActivity = m.now()
	}
	if session.closed {
		return os.ErrProcessDone
	}
	if session.controlOwner != client {
		return ErrControlNotOwner
	}
	if err := pty.Setsize(session.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}); err != nil {
		return err
	}
	session.meta.Cols, session.meta.Rows, session.meta.UpdatedAt = cols, rows, m.now().UTC()
	session.sequence++
	if err := m.worker.Resize(id, strconv.FormatUint(session.sequence, 10), cols, rows); err != nil {
		m.fail(err)
		return err
	}
	if m.hasPersistence() && !session.ephemeral {
		_ = m.saveMeta(session.meta)
	}
	session.scheduleSnapshotLocked()
	return nil
}

package auth

import "github.com/ben-wangz/roaminal/backend/internal/persistence"

// ConnectionInstanceOrder returns a copy of the current login session's saved
// sidebar order. A missing session has no saved preference.
func (m *Manager) ConnectionInstanceOrder(sessionID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.refresh[sessionID]
	if !ok {
		return nil
	}
	return append([]string(nil), entry.ConnectionInstanceOrder...)
}

// SetConnectionInstanceOrder persists the current login session's sidebar
// order without changing any token state.
func (m *Manager) SetConnectionInstanceOrder(sessionID string, order []string) error {
	if err := persistence.ValidateConnectionInstanceOrder(order); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.refresh[sessionID]
	if !ok {
		return ErrNotFound
	}
	previous := entry.ConnectionInstanceOrder
	entry.ConnectionInstanceOrder = append([]string(nil), order...)
	m.refresh[sessionID] = entry
	if err := m.persistLocked(); err != nil {
		entry.ConnectionInstanceOrder = previous
		m.refresh[sessionID] = entry
		return err
	}
	return nil
}

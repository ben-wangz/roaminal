package auth

import (
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// IsSessionActive distinguishes an unavailable check from confirmed inactivity
// so delivery fails closed and cleanup does not prune on outage.
func (m *Manager) IsSessionActive(sessionID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reconcileLocked(); err != nil {
		return false, err
	}
	return m.sessionUsableLocked(m.refresh[sessionID], m.clock.Now().UTC()), nil
}

// sessionUsableLocked only checks local state; it cannot authorize anything
// without a successful reconciliation.
func (m *Manager) sessionUsableLocked(entry domain.AuthSessionRecord, now time.Time) bool {
	return entry.RefreshExpiresAt.After(now) && entry.PasswordFingerprint == m.fingerprint && m.sessionBoundLocked(entry)
}

// Logout returns the revoked session ID for resource cleanup. Revoking a
// locally known credential does not require a live enrollment read.
func (m *Manager) Logout(refreshToken, accessToken string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, _ := m.refreshByTokenLocked(refreshToken)
	if id == "" && accessToken != "" {
		id = m.access[hashToken(accessToken)].SessionID
	}
	if id == "" {
		return "", nil
	}
	m.removeSessionLocked(id)
	return id, m.persistLocked()
}

func (m *Manager) checkSessionLocked(sessionID string) error {
	reconcileErr := m.reconcileLocked()
	if !m.sessionUsableLocked(m.refresh[sessionID], m.clock.Now().UTC()) {
		return ErrUnauthorized
	}
	return reconcileErr
}

func (m *Manager) Current(sessionID string) (CurrentSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkSessionLocked(sessionID); err != nil {
		return CurrentSession{}, err
	}
	entry := m.refresh[sessionID]
	var accessExpires time.Time
	for _, value := range m.access {
		if value.SessionID == sessionID && value.ExpiresAt.After(accessExpires) {
			accessExpires = value.ExpiresAt
		}
	}
	return CurrentSession{Authenticated: true, SessionID: sessionID, AccessTokenExpiresAt: accessExpires, RefreshTokenExpiresAt: entry.RefreshExpiresAt}, nil
}

func (m *Manager) List(sessionID string) ([]SessionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkSessionLocked(sessionID); err != nil {
		return nil, err
	}
	result := make([]SessionSummary, 0, len(m.refresh))
	now := m.clock.Now().UTC()
	changed := false
	for id, entry := range m.refresh {
		if !m.sessionUsableLocked(entry, now) {
			m.removeSessionLocked(id)
			changed = true
			continue
		}
		result = append(result, SessionSummary{ID: entry.ID, CreatedAt: entry.CreatedAt, LastSeenAt: entry.LastSeenAt, RefreshExpiresAt: entry.RefreshExpiresAt, UserAgent: entry.UserAgent, Current: entry.ID == sessionID})
	}
	if changed {
		_ = m.persistLocked()
	}
	return result, nil
}

func (m *Manager) Revoke(currentID, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkSessionLocked(currentID); err != nil {
		return err
	}
	if _, ok := m.refresh[sessionID]; !ok {
		return ErrNotFound
	}
	m.removeSessionLocked(sessionID)
	return m.persistLocked()
}

func (m *Manager) LogoutOthers(sessionID string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkSessionLocked(sessionID); err != nil {
		return nil, err
	}
	var revoked []string
	for id := range m.refresh {
		if id != sessionID {
			revoked = append(revoked, id)
			delete(m.refresh, id)
		}
	}
	for key, entry := range m.access {
		if entry.SessionID != sessionID {
			delete(m.access, key)
		}
	}
	return revoked, m.persistLocked()
}

func (m *Manager) removeSessionLocked(sessionID string) {
	delete(m.refresh, sessionID)
	for key, entry := range m.access {
		if entry.SessionID == sessionID {
			delete(m.access, key)
		}
	}
}

func (m *Manager) Locked() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.locked }

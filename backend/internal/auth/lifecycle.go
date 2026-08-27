package auth

import (
	"crypto/hmac"
	"strings"
	"time"
)

// IsSessionActive reports whether an authentication session can still be used
// for an authenticated resource. It is used by durable resource cleanup and
// does not mutate the session store.
func (m *Manager) IsSessionActive(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.refresh[sessionID]
	return ok && entry.RefreshExpiresAt.After(m.clock.Now().UTC()) && entry.PasswordFingerprint == m.fingerprint
}

func (m *Manager) SessionIDForRefresh(refreshToken string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := hashToken(strings.TrimSpace(refreshToken))
	for sessionID, entry := range m.refresh {
		if refreshToken != "" && hmac.Equal([]byte(entry.RefreshTokenHash), []byte(hash)) && entry.RefreshExpiresAt.After(m.clock.Now().UTC()) && entry.PasswordFingerprint == m.fingerprint {
			return sessionID, true
		}
	}
	return "", false
}

func (m *Manager) Logout(refreshToken, accessToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var id string
	hash := hashToken(strings.TrimSpace(refreshToken))
	for sessionID, entry := range m.refresh {
		if refreshToken != "" && hmac.Equal([]byte(entry.RefreshTokenHash), []byte(hash)) {
			id = sessionID
			break
		}
	}
	if id == "" && accessToken != "" {
		if entry, ok := m.access[hashToken(accessToken)]; ok {
			id = entry.SessionID
		}
	}
	if id == "" {
		return nil
	}
	delete(m.refresh, id)
	for key, entry := range m.access {
		if entry.SessionID == id {
			delete(m.access, key)
		}
	}
	return m.persistLocked()
}

func (m *Manager) Current(sessionID string) (CurrentSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.refresh[sessionID]
	if !ok || !entry.RefreshExpiresAt.After(m.clock.Now().UTC()) {
		if ok {
			delete(m.refresh, sessionID)
			_ = m.persistLocked()
		}
		return CurrentSession{}, ErrUnauthorized
	}
	var accessExpires time.Time
	for _, value := range m.access {
		if value.SessionID == sessionID && value.ExpiresAt.After(accessExpires) {
			accessExpires = value.ExpiresAt
		}
	}
	return CurrentSession{Authenticated: true, SessionID: sessionID, AccessTokenExpiresAt: accessExpires, RefreshTokenExpiresAt: entry.RefreshExpiresAt}, nil
}

func (m *Manager) List(sessionID string) []SessionSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]SessionSummary, 0, len(m.refresh))
	now := m.clock.Now().UTC()
	changed := false
	for id, entry := range m.refresh {
		if !entry.RefreshExpiresAt.After(now) {
			delete(m.refresh, id)
			changed = true
			continue
		}
		result = append(result, SessionSummary{ID: entry.ID, CreatedAt: entry.CreatedAt, LastSeenAt: entry.LastSeenAt, RefreshExpiresAt: entry.RefreshExpiresAt, UserAgent: entry.UserAgent, Current: entry.ID == sessionID})
	}
	if changed {
		_ = m.persistLocked()
	}
	return result
}

func (m *Manager) Revoke(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.refresh[sessionID]; !ok {
		return ErrNotFound
	}
	delete(m.refresh, sessionID)
	for key, entry := range m.access {
		if entry.SessionID == sessionID {
			delete(m.access, key)
		}
	}
	return m.persistLocked()
}

func (m *Manager) LogoutOthers(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.refresh {
		if id != sessionID {
			delete(m.refresh, id)
		}
	}
	for key, entry := range m.access {
		if entry.SessionID != sessionID {
			delete(m.access, key)
		}
	}
	return m.persistLocked()
}

func (m *Manager) Locked() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.locked }

package auth

import (
	"crypto/hmac"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// issueLocked mints normal credentials. It is reachable only after a
// completed TOTP verification, so every issued session is bound to the
// current enrollment identity; refresh sessions from any other identity are
// rejected on every later boundary.
func (m *Manager) issueLocked(userAgent string) (Tokens, error) {
	if m.enrollment == nil || m.enrollmentUnreadable || m.enrollmentInvalidationPending {
		return Tokens{}, ErrNotConfigured
	}
	now := m.clock.Now().UTC()
	sessionID, err := m.ids.NewID()
	if err != nil {
		return Tokens{}, err
	}
	access, err := opaque(m.random, "ra")
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := opaque(m.random, "rr")
	if err != nil {
		return Tokens{}, err
	}
	entry := domain.AuthSessionRecord{ID: sessionID, PasswordFingerprint: m.fingerprint, EnrollmentID: m.enrollment.EnrollmentID, RefreshTokenHash: hashToken(refresh), CreatedAt: now, LastSeenAt: now, RefreshExpiresAt: now.Add(m.cfg.AuthRefreshTTL), RotatedAt: now, UserAgent: truncate(userAgent, 500)}
	m.refresh[sessionID] = entry
	accessHash := hashToken(access)
	m.access[accessHash] = accessEntry{SessionID: sessionID, ExpiresAt: now.Add(m.cfg.AuthAccessTTL)}
	if err := m.persistLocked(); err != nil {
		delete(m.refresh, sessionID)
		delete(m.access, accessHash)
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, AccessTokenExpiresAt: now.Add(m.cfg.AuthAccessTTL), RefreshToken: refresh, RefreshTokenExpiresAt: entry.RefreshExpiresAt, SessionID: sessionID}, nil
}

// Authenticate validates a bearer access token. It reconciles enrollment
// state first so a deleted enrollment denies business access immediately
// without a restart.
func (m *Manager) Authenticate(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reconcileErr := m.reconcileLocked()
	entry := m.access[hashToken(strings.TrimSpace(token))]
	now := m.clock.Now().UTC()
	if !entry.ExpiresAt.After(now) || !m.sessionUsableLocked(m.refresh[entry.SessionID], now) {
		return "", ErrUnauthorized
	}
	if reconcileErr != nil {
		return "", reconcileErr
	}
	return entry.SessionID, nil
}

// Refresh rotates an existing session. Refresh is allowed only for a valid
// session bound to the current enrollment; it never requires a new TOTP
// code, and pre-upgrade or stale-identity refresh tokens are rejected.
func (m *Manager) Refresh(token, userAgent string) (Tokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reconcileErr := m.reconcileLocked()
	sessionID, old := m.refreshByTokenLocked(token)
	if !m.sessionUsableLocked(old, m.clock.Now().UTC()) {
		return Tokens{}, ErrUnauthorized
	}
	if reconcileErr != nil {
		return Tokens{}, reconcileErr
	}
	previous := old
	oldAccess := make(map[string]accessEntry)
	for key, entry := range m.access {
		if entry.SessionID == sessionID {
			oldAccess[key] = entry
			delete(m.access, key)
		}
	}
	delete(m.refresh, sessionID)
	access, err := opaque(m.random, "ra")
	if err != nil {
		m.refresh[sessionID] = previous
		for key, entry := range oldAccess {
			m.access[key] = entry
		}
		return Tokens{}, err
	}
	refresh, err := opaque(m.random, "rr")
	if err != nil {
		m.refresh[sessionID] = previous
		for key, entry := range oldAccess {
			m.access[key] = entry
		}
		return Tokens{}, err
	}
	now := m.clock.Now().UTC()
	old.RefreshTokenHash, old.LastSeenAt, old.RotatedAt = hashToken(refresh), now, now
	if userAgent != "" {
		old.UserAgent = truncate(userAgent, 500)
	}
	m.refresh[sessionID] = old
	accessHash := hashToken(access)
	m.access[accessHash] = accessEntry{SessionID: sessionID, ExpiresAt: now.Add(m.cfg.AuthAccessTTL)}
	if err := m.persistLocked(); err != nil {
		delete(m.refresh, sessionID)
		delete(m.access, accessHash)
		m.refresh[sessionID] = previous
		for key, entry := range oldAccess {
			m.access[key] = entry
		}
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, AccessTokenExpiresAt: now.Add(m.cfg.AuthAccessTTL), RefreshToken: refresh, RefreshTokenExpiresAt: old.RefreshExpiresAt, SessionID: sessionID}, nil
}

func (m *Manager) refreshByTokenLocked(token string) (string, domain.AuthSessionRecord) {
	token = strings.TrimSpace(token)
	if token != "" {
		hash := hashToken(token)
		for id, entry := range m.refresh {
			if hmac.Equal([]byte(entry.RefreshTokenHash), []byte(hash)) {
				return id, entry
			}
		}
	}
	return "", domain.AuthSessionRecord{}
}

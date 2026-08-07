package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func (m *Manager) Login(challengeID, response, userAgent string) (Tokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.challenges[challengeID]
	delete(m.challenges, challengeID)
	if !ok || !c.ExpiresAt.After(time.Now().UTC()) {
		return Tokens{}, ErrInvalidChallenge
	}
	if m.locked {
		return Tokens{}, ErrLocked
	}
	h := hmac.New(sha256.New, passwordKey(m.cfg.Password))
	_, _ = h.Write([]byte(loginMessage(c)))
	provided, err := hex.DecodeString(strings.TrimSpace(response))
	if err != nil || len(provided) != h.Size() || !hmac.Equal(provided, h.Sum(nil)) {
		m.failedAttempts++
		if m.failedAttempts >= m.cfg.AuthMaxAttempts {
			m.locked = true
			return Tokens{}, ErrLocked
		}
		return Tokens{}, ErrUnauthorized
	}
	m.failedAttempts = 0
	return m.issueLocked(userAgent)
}

func (m *Manager) issueLocked(userAgent string) (Tokens, error) {
	now := time.Now().UTC()
	sessionID, err := newID()
	if err != nil {
		return Tokens{}, err
	}
	access, err := opaque("ra")
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := opaque("rr")
	if err != nil {
		return Tokens{}, err
	}
	entry := persistence.AuthSession{ID: sessionID, PasswordFingerprint: m.fingerprint, RefreshTokenHash: hashToken(refresh), CreatedAt: now, LastSeenAt: now, RefreshExpiresAt: now.Add(m.cfg.AuthRefreshTTL), RotatedAt: now, UserAgent: truncate(userAgent, 500)}
	m.refresh[sessionID] = entry
	m.access[hashToken(access)] = accessEntry{SessionID: sessionID, ExpiresAt: now.Add(m.cfg.AuthAccessTTL)}
	if err := m.persistLocked(); err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, AccessTokenExpiresAt: now.Add(m.cfg.AuthAccessTTL), RefreshToken: refresh, RefreshTokenExpiresAt: entry.RefreshExpiresAt, SessionID: sessionID}, nil
}

func (m *Manager) Authenticate(token string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.access[hashToken(strings.TrimSpace(token))]
	if !ok || !entry.ExpiresAt.After(time.Now().UTC()) {
		return "", ErrUnauthorized
	}
	return entry.SessionID, nil
}

func (m *Manager) Refresh(token, userAgent string) (Tokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := hashToken(strings.TrimSpace(token))
	var old persistence.AuthSession
	var sessionID string
	for id, entry := range m.refresh {
		if hmac.Equal([]byte(entry.RefreshTokenHash), []byte(hash)) {
			old, sessionID = entry, id
			break
		}
	}
	if sessionID == "" || !old.RefreshExpiresAt.After(time.Now().UTC()) || old.PasswordFingerprint != m.fingerprint {
		return Tokens{}, ErrUnauthorized
	}
	delete(m.refresh, sessionID)
	for key, entry := range m.access {
		if entry.SessionID == sessionID {
			delete(m.access, key)
		}
	}
	access, err := opaque("ra")
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := opaque("rr")
	if err != nil {
		return Tokens{}, err
	}
	now := time.Now().UTC()
	old.RefreshTokenHash, old.LastSeenAt, old.RotatedAt = hashToken(refresh), now, now
	if userAgent != "" {
		old.UserAgent = truncate(userAgent, 500)
	}
	m.refresh[sessionID] = old
	m.access[hashToken(access)] = accessEntry{SessionID: sessionID, ExpiresAt: now.Add(m.cfg.AuthAccessTTL)}
	if err := m.persistLocked(); err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, AccessTokenExpiresAt: now.Add(m.cfg.AuthAccessTTL), RefreshToken: refresh, RefreshTokenExpiresAt: old.RefreshExpiresAt, SessionID: sessionID}, nil
}

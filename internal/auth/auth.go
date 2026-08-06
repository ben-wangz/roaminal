package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/internal/config"
	"github.com/ben-wangz/roaminal/internal/persistence"
)

const (
	ChallengeTTL  = 30 * time.Second
	AuthAlgorithm = "roaminal-hmac-sha256-login-v1"
	MessagePrefix = "roaminal-login-v1:"
)

var (
	ErrInvalidChallenge = errors.New("invalid login challenge")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrLocked           = errors.New("service locked")
	ErrNotFound         = errors.New("auth session not found")
)

type ChallengeResponse struct {
	ChallengeID string    `json:"challengeId"`
	Salt        string    `json:"salt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Algorithm   string    `json:"algorithm"`
}

type Tokens struct {
	AccessToken           string    `json:"accessToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshToken          string    `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
	SessionID             string    `json:"-"`
}

type CurrentSession struct {
	Authenticated         bool      `json:"authenticated"`
	SessionID             string    `json:"sessionId"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

type SessionSummary struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"createdAt"`
	LastSeenAt       time.Time `json:"lastSeenAt"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	UserAgent        string    `json:"userAgent"`
	Current          bool      `json:"current"`
}

type accessEntry struct {
	SessionID string
	ExpiresAt time.Time
}

type challenge struct {
	ID        string
	Salt      string
	ExpiresAt time.Time
}

type Manager struct {
	mu             sync.Mutex
	cfg            config.Config
	store          *persistence.Store
	fingerprint    string
	refresh        map[string]persistence.AuthSession
	access         map[string]accessEntry
	challenges     map[string]challenge
	failedAttempts int
	locked         bool
}

func New(cfg config.Config, store *persistence.Store) (*Manager, error) {
	passwordKey := sha256.Sum256([]byte(cfg.Password))
	fingerprintHash := sha256.Sum256(append([]byte("roaminal-password-fingerprint-v1:"), passwordKey[:]...))
	m := &Manager{
		cfg: cfg, store: store, fingerprint: hex.EncodeToString(fingerprintHash[:]),
		refresh: make(map[string]persistence.AuthSession), access: make(map[string]accessEntry),
		challenges: make(map[string]challenge),
	}
	file, err := store.LoadAuth()
	if err != nil {
		return nil, err
	}
	changed := false
	now := time.Now().UTC()
	for _, entry := range file.Sessions {
		if entry.PasswordFingerprint != m.fingerprint || !entry.RefreshExpiresAt.After(now) {
			changed = true
			continue
		}
		m.refresh[entry.ID] = entry
	}
	if changed {
		if err := m.persistLocked(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Manager) Fingerprint() string { return m.fingerprint }

func (m *Manager) persistLocked() error {
	entries := make([]persistence.AuthSession, 0, len(m.refresh))
	for _, entry := range m.refresh {
		entries = append(entries, entry)
	}
	return m.store.SaveAuth(persistence.AuthFile{Sessions: entries})
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:]), nil
}

func opaque(prefix string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func passwordKey(password string) []byte { sum := sha256.Sum256([]byte(password)); return sum[:] }

func (m *Manager) Challenge() (ChallengeResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, err := newID()
	if err != nil {
		return ChallengeResponse{}, err
	}
	var saltBytes [32]byte
	if _, err := rand.Read(saltBytes[:]); err != nil {
		return ChallengeResponse{}, err
	}
	expires := time.Now().UTC().Add(ChallengeTTL)
	challenge := challenge{ID: id, Salt: base64.RawURLEncoding.EncodeToString(saltBytes[:]), ExpiresAt: expires}
	m.challenges[id] = challenge
	for key, value := range m.challenges {
		if !value.ExpiresAt.After(time.Now()) {
			delete(m.challenges, key)
		}
	}
	return ChallengeResponse{ChallengeID: id, Salt: challenge.Salt, ExpiresAt: expires, Algorithm: AuthAlgorithm}, nil
}

func Proof(password string, c ChallengeResponse) string {
	message := MessagePrefix + c.ChallengeID + ":" + c.Salt + ":" + c.ExpiresAt.UTC().Format(time.RFC3339Nano)
	h := hmac.New(sha256.New, passwordKey(password))
	_, _ = h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

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
	message := MessagePrefix + c.ID + ":" + c.Salt + ":" + c.ExpiresAt.UTC().Format(time.RFC3339Nano)
	h := hmac.New(sha256.New, passwordKey(m.cfg.Password))
	_, _ = h.Write([]byte(message))
	expected := h.Sum(nil)
	provided, err := hex.DecodeString(strings.TrimSpace(response))
	if err != nil || len(provided) != len(expected) || !hmac.Equal(provided, expected) {
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

func truncate(value string, max int) string {
	data := []byte(strings.TrimSpace(value))
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max])
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
	old.RefreshTokenHash = hashToken(refresh)
	old.LastSeenAt = now
	old.RotatedAt = now
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

func (m *Manager) Logout(refreshToken string, accessToken string) error {
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
	if !ok {
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
	for _, entry := range m.refresh {
		result = append(result, SessionSummary{ID: entry.ID, CreatedAt: entry.CreatedAt, LastSeenAt: entry.LastSeenAt, RefreshExpiresAt: entry.RefreshExpiresAt, UserAgent: entry.UserAgent, Current: entry.ID == sessionID})
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

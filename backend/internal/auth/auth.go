package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func New(cfg config.Config, store *persistence.Store) (*Manager, error) {
	passwordKey := sha256.Sum256([]byte(cfg.Password))
	fingerprintHash := sha256.Sum256(append([]byte("roaminal-password-fingerprint-v1:"), passwordKey[:]...))
	m := &Manager{cfg: cfg, store: store, fingerprint: hex.EncodeToString(fingerprintHash[:]), refresh: make(map[string]persistence.AuthSession), access: make(map[string]accessEntry), challenges: make(map[string]challenge)}
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

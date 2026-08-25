package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func NewWithRepositories(cfg config.Config, authRepository ports.AuthRepository, deps Dependencies) (*Manager, error) {
	if authRepository == nil {
		return nil, errors.New("auth repository is required")
	}
	if deps.Clock == nil {
		return nil, errors.New("auth clock is required")
	}
	if deps.IDs == nil {
		return nil, errors.New("auth id generator is required")
	}
	if deps.Random == nil {
		return nil, errors.New("auth random source is required")
	}
	passwordKey := sha256.Sum256([]byte(cfg.Password))
	fingerprintHash := sha256.Sum256(append([]byte("roaminal-password-fingerprint-v1:"), passwordKey[:]...))
	m := &Manager{cfg: cfg, authRepository: authRepository, clock: deps.Clock, ids: deps.IDs, random: deps.Random, fingerprint: hex.EncodeToString(fingerprintHash[:]), refresh: make(map[string]domain.AuthSessionRecord), access: make(map[string]accessEntry), challenges: make(map[string]challenge)}
	records, err := authRepository.LoadAuth(context.Background())
	if err != nil {
		return nil, err
	}
	changed := false
	now := m.clock.Now().UTC()
	for _, entry := range records {
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
	entries := make([]domain.AuthSessionRecord, 0, len(m.refresh))
	for _, entry := range m.refresh {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return m.authRepository.SaveAuth(context.Background(), entries)
}

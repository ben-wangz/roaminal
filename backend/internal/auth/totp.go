package auth

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// enrollmentCheckContext bounds each live state check so a stalled filesystem
// cannot hold the authentication mutex indefinitely.
func (m *Manager) enrollmentCheckContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), EnrollmentCheckTimeout)
}

// loadEnrollmentLocked reads the persistent enrollment with a bounded check.
func (m *Manager) loadEnrollmentLocked() (domain.TOTPEnrollmentRecord, bool, error) {
	ctx, cancel := m.enrollmentCheckContext()
	defer cancel()
	record, exists, err := m.enrollmentRepo.LoadEnrollment(ctx)
	if err != nil {
		return domain.TOTPEnrollmentRecord{}, true, err
	}
	return record, exists, nil
}

// reconcileLocked is the single state reconciler. Every authentication
// boundary calls it so enrollment deletion, restoration, or corruption is
// detected without a restart and never trusts an indefinitely cached secret.
func (m *Manager) reconcileLocked() {
	record, exists, err := m.loadEnrollmentLocked()
	if err != nil {
		if m.enrollmentState != enrollmentStorageError {
			m.enrollmentState = enrollmentStorageError
			_ = m.invalidateCredentialsLocked()
			// Report the unusable file server-side; the message never
			// contains secret material.
			log.Printf("level=ERROR event=auth_enrollment_storage_denied error=%v", err)
		}
		return
	}
	switch {
	case !exists:
		if m.enrollmentState != enrollmentUnconfigured {
			m.enrollmentState = enrollmentUnconfigured
			m.enrollment = nil
			if err := m.invalidateCredentialsLocked(); err != nil {
				// The in-memory credentials are already gone, but do not offer
				// enrollment while durable invalidation is unavailable.
				m.enrollmentState = enrollmentStorageError
			}
		}
	case m.enrollmentState == enrollmentConfigured && m.enrollment != nil && record == *m.enrollment:
		// Same enrollment record. Time-step consumption is written
		// synchronously before credentials are granted, so the cache stays
		// authoritative and no reread is needed.
	default:
		// A new enrollment identity appeared (first durable enrollment,
		// restored backup, or repair after a storage error). All previously
		// issued credentials and pending stages belong to the old state.
		m.enrollmentState = enrollmentConfigured
		enrollment := record
		m.enrollment = &enrollment
		if err := m.invalidateCredentialsLocked(); err != nil {
			m.enrollmentState = enrollmentStorageError
		}
	}
}

// invalidateCredentialsLocked clears pending login/setup credentials, access
// tokens, and refresh sessions of the previous enrollment state, persists the
// invalidation, and wakes stream watchers. Callers set the new enrollment
// state and cached record themselves before or right after this call; this
// helper never touches them.
func (m *Manager) invalidateCredentialsLocked() error {
	m.pending = make(map[string]pendingEntry)
	m.access = make(map[string]accessEntry)
	m.challenges = make(map[string]challenge)
	m.refresh = make(map[string]domain.AuthSessionRecord)
	if err := m.persistLocked(); err != nil {
		// Access stays denied: the in-memory clear above plus enrollment
		// binding reject every old credential even if this write failed.
		// Surface the failure server-side without leaking record contents.
		log.Printf("level=ERROR event=auth_session_invalidation_failed error_type=%T", err)
		if m.enrollmentChange != nil {
			close(m.enrollmentChange)
			m.enrollmentChange = make(chan struct{})
		}
		return err
	}
	if m.enrollmentChange != nil {
		close(m.enrollmentChange)
		m.enrollmentChange = make(chan struct{})
	}
	return nil
}

// EnrollmentChanged returns a channel that is closed whenever enrollment
// identity changes, the enrollment file is deleted, or its storage turns
// unusable. Long-lived streams watch it to disconnect promptly.
func (m *Manager) EnrollmentChanged() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enrollmentChange
}

// StartEnrollmentWatch reconciles enrollment state on a short interval so
// already-open connections cannot stay authorized after the secret file is
// deleted from the running backend.
func (m *Manager) StartEnrollmentWatch(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				m.reconcileLocked()
				m.mu.Unlock()
			}
		}
	}()
}

// sessionBoundLocked reports whether a persisted session belongs to the
// currently enrolled identity. Sessions without a binding are legacy
// pre-TOTP credentials and are always rejected.
func (m *Manager) sessionBoundLocked(entry domain.AuthSessionRecord) bool {
	return m.enrollmentState == enrollmentConfigured && m.enrollment != nil && entry.EnrollmentID == m.enrollment.EnrollmentID
}

// newPendingLocked creates a purpose-bound short-lived pending credential
// keyed by token hash. Pending tokens are never accepted as bearer or
// refresh credentials; only the exact nextStep exchange consumes them.
func (m *Manager) newPendingLocked(purpose pendingPurpose, enrollmentID, userAgent string) (string, time.Time, error) {
	token, err := opaque(m.random, "rp")
	if err != nil {
		return "", time.Time{}, err
	}
	expires := m.clock.Now().UTC().Add(PendingTTL)
	m.prunePendingLocked()
	if len(m.pending) >= MaxPending {
		oldestKey, oldestExpiry := "", time.Time{}
		for key, entry := range m.pending {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = key, entry.expiresAt
			}
		}
		if oldestKey != "" {
			delete(m.pending, oldestKey)
		}
	}
	m.pending[hashToken(token)] = pendingEntry{purpose: purpose, expiresAt: expires, enrollmentID: enrollmentID, userAgent: truncate(userAgent, 500)}
	return token, expires, nil
}

func (m *Manager) prunePendingLocked() {
	now := m.clock.Now().UTC()
	for key, entry := range m.pending {
		if !entry.expiresAt.After(now) {
			delete(m.pending, key)
		}
	}
}

func (m *Manager) pendingLocked(token string) (pendingEntry, string, bool) {
	m.prunePendingLocked()
	hash := hashToken(strings.TrimSpace(token))
	entry, ok := m.pending[hash]
	if !ok {
		return pendingEntry{}, "", false
	}
	return entry, hash, true
}

// recordPendingFailureLocked applies per-token and global failure accounting
// for a wrong or replayed TOTP code. A valid password alone never resets the
// global budget; only completed authentication or enrollment does.
func (m *Manager) recordPendingFailureLocked(hash string, entry pendingEntry) error {
	entry.attempts++
	if entry.attempts >= MaxPendingAttempts {
		delete(m.pending, hash)
	} else {
		m.pending[hash] = entry
	}
	m.failedAttempts++
	if m.failedAttempts >= m.cfg.AuthMaxAttempts {
		m.locked = true
		return ErrLocked
	}
	return ErrUnauthorized
}

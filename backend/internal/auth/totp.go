package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

var errEnrollmentReplayRegression = errors.New("TOTP replay checkpoint moved backwards")

// enrollmentCheckContext bounds live enrollment repository operations.
func (m *Manager) enrollmentCheckContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), EnrollmentCheckTimeout)
}

// loadEnrollmentLocked reads the persistent enrollment with a bounded check.
func (m *Manager) loadEnrollmentLocked() (domain.TOTPEnrollmentRecord, bool, error) {
	ctx, cancel := m.enrollmentCheckContext()
	defer cancel()
	return m.enrollmentRepo.LoadEnrollment(ctx)
}

// reconcileLocked must succeed before any credential can authorize access.
// A failed read proves neither deletion nor change; keep the last verified
// record and credentials to compare against a subsequent successful read.
func (m *Manager) reconcileLocked() error {
	record, exists, err := m.loadEnrollmentLocked()
	same := exists && sameEnrollmentIdentity(m.enrollment, &record)
	if err == nil && same && record.LastAcceptedTimeStep < m.enrollment.LastAcceptedTimeStep {
		// Restoring a lower replay checkpoint for the same secret must not
		// make an already accepted code usable while this runtime is serving.
		err = errEnrollmentReplayRegression
	}
	if err != nil {
		if !m.enrollmentUnreadable {
			m.enrollmentUnreadable = true
			m.signalEnrollmentChangeLocked()
			log.Printf("level=ERROR event=auth_enrollment_storage_denied error_type=%T replay_regressed=%t", err, errors.Is(err, errEnrollmentReplayRegression))
		}
		// A previously confirmed change still needs a durable wipe, even
		// when the latest read alone would not justify deleting anything.
		_ = m.finishEnrollmentInvalidationLocked()
		return ErrEnrollmentStorage
	}
	m.enrollmentUnreadable = false
	switch {
	case !exists && m.enrollment != nil:
		m.replaceEnrollmentLocked(nil)
	case exists && !same:
		m.replaceEnrollmentLocked(&record)
	case same && record.LastAcceptedTimeStep > m.enrollment.LastAcceptedTimeStep:
		// A replay checkpoint may have committed even if the caller did not
		// receive the write response. Progress does not change the identity.
		m.enrollment = &record
	}
	return m.finishEnrollmentInvalidationLocked()
}

func sameEnrollmentIdentity(a, b *domain.TOTPEnrollmentRecord) bool {
	return a != nil && b != nil && a.FormatVersion == b.FormatVersion && a.EnrollmentID == b.EnrollmentID && a.Secret == b.Secret
}

func (m *Manager) replaceEnrollmentLocked(record *domain.TOTPEnrollmentRecord) {
	m.enrollment = record
	clear(m.pending)
	clear(m.access)
	clear(m.challenges)
	clear(m.refresh)
	m.enrollmentInvalidationPending = true
	m.signalEnrollmentChangeLocked()
}

// A confirmed change invalidates credentials immediately, even if storage
// writes fail or an old enrollment is restored. Do not permit new enrollment
// or login until the durable invalidation also succeeds.
func (m *Manager) finishEnrollmentInvalidationLocked() error {
	if !m.enrollmentInvalidationPending {
		return nil
	}
	ctx, cancel := m.enrollmentCheckContext()
	defer cancel()
	if err := m.authRepository.SaveAuth(ctx, nil); err != nil {
		log.Printf("level=ERROR event=auth_session_invalidation_failed error_type=%T", err)
		return ErrEnrollmentStorage
	}
	m.enrollmentInvalidationPending = false
	return nil
}

func (m *Manager) signalEnrollmentChangeLocked() {
	close(m.enrollmentChange)
	m.enrollmentChange = make(chan struct{})
}

// EnrollmentChanged returns a channel that is closed on a verified change
// or when enrollment becomes unreadable. Streams use it to disconnect promptly.
func (m *Manager) EnrollmentChanged() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enrollmentChange
}

// StartEnrollmentWatch reconciles enrollment periodically so idle streams
// disconnect when enrollment changes or becomes unverifiable. The returned
// channel closes after cancellation and any in-progress reconciliation.
func (m *Manager) StartEnrollmentWatch(ctx context.Context, interval time.Duration) <-chan struct{} {
	if interval <= 0 {
		interval = time.Second
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				_ = m.reconcileLocked()
				m.mu.Unlock()
			}
		}
	}()
	return done
}

// sessionBoundLocked reports whether a persisted session belongs to the
// currently enrolled identity. Sessions without a binding are legacy
// pre-TOTP credentials and are always rejected.
func (m *Manager) sessionBoundLocked(entry domain.AuthSessionRecord) bool {
	return m.enrollment != nil && entry.EnrollmentID == m.enrollment.EnrollmentID
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

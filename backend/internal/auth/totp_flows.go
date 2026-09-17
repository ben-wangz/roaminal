package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// Login verifies the password proof and returns a restricted pending
// credential for the mandatory TOTP stage. It never issues normal tokens:
// before enrollment the pending token may only drive TOTP setup, after
// enrollment only TOTP verification.
func (m *Manager) Login(challengeID, response, userAgent string) (LoginPending, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Claim the challenge before reconciling: a password challenge is a
	// proof vehicle, not an enrollment credential, so an in-flight login
	// survives an enrollment transition that happened after the challenge
	// was issued.
	c, ok := m.challenges[challengeID]
	delete(m.challenges, challengeID)
	if !ok || !c.ExpiresAt.After(m.clock.Now().UTC()) {
		return LoginPending{}, ErrInvalidChallenge
	}
	if m.locked {
		return LoginPending{}, ErrLocked
	}
	m.reconcileLocked()
	if m.enrollmentState == enrollmentStorageError {
		return LoginPending{}, ErrEnrollmentStorage
	}
	if !verifyProof(m.cfg.Password, c, response) {
		m.failedAttempts++
		if m.failedAttempts >= m.cfg.AuthMaxAttempts {
			m.locked = true
			return LoginPending{}, ErrLocked
		}
		return LoginPending{}, ErrUnauthorized
	}
	// The password proof is accepted but the global failure budget stays:
	// a valid password must not reset TOTP failure accounting, and a fresh
	// pending token must not bypass it.
	if m.enrollmentState == enrollmentConfigured {
		token, expires, err := m.newPendingLocked(pendingVerify, m.enrollment.EnrollmentID, userAgent)
		if err != nil {
			return LoginPending{}, err
		}
		return LoginPending{NextStep: LoginNextStepVerify, PendingToken: token, ExpiresAt: expires}, nil
	}
	token, expires, err := m.newPendingLocked(pendingSetup, "", userAgent)
	if err != nil {
		return LoginPending{}, err
	}
	return LoginPending{NextStep: LoginNextStepSetup, PendingToken: token, ExpiresAt: expires}, nil
}

func verifyProof(password string, c challenge, response string) bool {
	h := hmac.New(sha256.New, passwordKey(password))
	_, _ = h.Write([]byte(loginMessage(c)))
	provided, err := hex.DecodeString(strings.TrimSpace(response))
	return err == nil && len(provided) == h.Size() && hmac.Equal(provided, h.Sum(nil))
}

// Setup returns the enrollment candidate for a setup-only pending token.
// Repeated requests for one valid pending token return the same candidate
// until expiry so the shown QR and secret can never drift apart.
func (m *Manager) Setup(pendingToken string) (TOTPSetup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileLocked()
	if m.enrollmentState == enrollmentStorageError {
		return TOTPSetup{}, ErrEnrollmentStorage
	}
	if m.locked {
		return TOTPSetup{}, ErrLocked
	}
	if m.enrollmentState == enrollmentConfigured {
		return TOTPSetup{}, ErrAlreadyConfigured
	}
	entry, hash, ok := m.pendingLocked(pendingToken)
	if !ok || entry.purpose != pendingSetup {
		return TOTPSetup{}, ErrInvalidPending
	}
	if entry.candidate == nil {
		candidate, err := m.newCandidateLocked()
		if err != nil {
			return TOTPSetup{}, err
		}
		entry.candidate = &candidate
		m.pending[hash] = entry
	}
	return TOTPSetup{ProvisioningURI: entry.candidate.uri, Secret: entry.candidate.secret, QRImage: entry.candidate.qrPNG, ExpiresAt: entry.expiresAt}, nil
}

// ConfirmSetup validates the confirmation code against the setup candidate,
// persists the enrollment durably, and invalidates every existing credential
// and competing setup attempt. The first successful confirmation wins.
func (m *Manager) ConfirmSetup(pendingToken, code string) (TOTPConfirmResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileLocked()
	if m.locked {
		return TOTPConfirmResult{}, ErrLocked
	}
	if m.enrollmentState == enrollmentStorageError {
		return TOTPConfirmResult{}, ErrEnrollmentStorage
	}
	if m.enrollmentState == enrollmentConfigured {
		// Another confirmation already won; this pending stage is dead.
		return TOTPConfirmResult{}, ErrAlreadyConfigured
	}
	entry, hash, ok := m.pendingLocked(pendingToken)
	if !ok || entry.purpose != pendingSetup || entry.candidate == nil {
		return TOTPConfirmResult{}, ErrInvalidPending
	}
	step, valid := validateTOTPCode(entry.candidate.secret, code, m.clock.Now())
	if !valid {
		return TOTPConfirmResult{}, m.recordPendingFailureLocked(hash, entry)
	}
	enrollmentID, err := m.ids.NewID()
	if err != nil {
		return TOTPConfirmResult{}, err
	}
	record := domain.TOTPEnrollmentRecord{FormatVersion: domain.TOTPFormatVersion, EnrollmentID: enrollmentID, Secret: entry.candidate.secret, LastAcceptedTimeStep: step}
	// Durable enrollment first: success is never returned before the
	// enrollment is on disk, and the enrollment binding alone is enough to
	// reject every pre-existing session even if a crash happens before the
	// session invalidation write below.
	if err := m.saveEnrollmentLocked(record); err != nil {
		// Do not let an enrollment write failure look like a bad user code.
		// The caller must keep the service fail-closed and offer repair/remove
		// guidance through the stable storage error response.
		return TOTPConfirmResult{}, fmt.Errorf("persist TOTP enrollment: %w", ErrEnrollmentStorage)
	}
	// Every pending setup credential, access token, and refresh session
	// dies with the completed enrollment; the first confirmation wins.
	if err := m.adoptEnrollmentLocked(record); err != nil {
		return TOTPConfirmResult{}, fmt.Errorf("invalidate authentication sessions: %w", ErrEnrollmentStorage)
	}
	m.failedAttempts = 0
	return TOTPConfirmResult{ReauthenticationRequired: true}, nil
}

// VerifyTOTP completes a configured login: it validates the code against the
// enrolled secret, rejects replayed time steps, persists the consumed step
// before issuing credentials, and returns the normal token response.
func (m *Manager) VerifyTOTP(pendingToken, code, userAgent string) (Tokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileLocked()
	if m.locked {
		return Tokens{}, ErrLocked
	}
	if m.enrollmentState == enrollmentStorageError {
		return Tokens{}, ErrEnrollmentStorage
	}
	if m.enrollmentState != enrollmentConfigured {
		return Tokens{}, ErrInvalidPending
	}
	entry, hash, ok := m.pendingLocked(pendingToken)
	if !ok || entry.purpose != pendingVerify || entry.enrollmentID != m.enrollment.EnrollmentID {
		return Tokens{}, ErrInvalidPending
	}
	step, valid := validateTOTPCode(m.enrollment.Secret, code, m.clock.Now())
	if !valid {
		return Tokens{}, m.recordPendingFailureLocked(hash, entry)
	}
	if step <= m.enrollment.LastAcceptedTimeStep {
		// The time step was already consumed; codes for it stay invalid
		// across attempts, restarts, and concurrent verifications.
		return Tokens{}, m.recordPendingFailureLocked(hash, entry)
	}
	// Recheck the file before writing so a deleted enrollment is never
	// recreated merely to persist a cached time step.
	current, exists, err := m.loadEnrollmentLocked()
	if err != nil {
		m.enrollmentState = enrollmentStorageError
		m.invalidateCredentialsLocked()
		return Tokens{}, ErrEnrollmentStorage
	}
	if !exists || current != *m.enrollment {
		if exists {
			if err := m.adoptEnrollmentLocked(current); err != nil {
				return Tokens{}, ErrEnrollmentStorage
			}
		} else {
			if err := m.dropEnrollmentLocked(); err != nil {
				return Tokens{}, ErrEnrollmentStorage
			}
		}
		return Tokens{}, ErrInvalidPending
	}
	current.LastAcceptedTimeStep = step
	if err := m.saveEnrollmentLocked(current); err != nil {
		return Tokens{}, ErrEnrollmentStorage
	}
	enrollment := current
	m.enrollment = &enrollment
	delete(m.pending, hash)
	tokens, err := m.issueLocked(userAgent)
	if err != nil {
		return Tokens{}, err
	}
	// Completed authentication is the only point that resets the shared
	// password/TOTP failure budget.
	m.failedAttempts = 0
	return tokens, nil
}

func (m *Manager) saveEnrollmentLocked(record domain.TOTPEnrollmentRecord) error {
	ctx, cancel := m.enrollmentCheckContext()
	defer cancel()
	return m.enrollmentRepo.SaveEnrollment(ctx, record)
}

// adoptEnrollmentLocked switches to a newly durable enrollment identity and
// invalidates every credential issued for any previous state.
func (m *Manager) adoptEnrollmentLocked(record domain.TOTPEnrollmentRecord) error {
	enrollment := record
	m.enrollment = &enrollment
	m.enrollmentState = enrollmentConfigured
	if err := m.invalidateCredentialsLocked(); err != nil {
		m.enrollmentState = enrollmentStorageError
		return err
	}
	return nil
}

// dropEnrollmentLocked switches to the unconfigured state and invalidates
// every credential issued for the deleted enrollment.
func (m *Manager) dropEnrollmentLocked() error {
	m.enrollment = nil
	m.enrollmentState = enrollmentUnconfigured
	if err := m.invalidateCredentialsLocked(); err != nil {
		m.enrollmentState = enrollmentStorageError
		return err
	}
	return nil
}

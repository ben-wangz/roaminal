package auth

import (
	"errors"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

const (
	ChallengeTTL  = 30 * time.Second
	AuthAlgorithm = "roaminal-hmac-sha256-login-v1"
	MessagePrefix = "roaminal-login-v1:"
	// PendingTTL bounds password-proven login stages that still owe a TOTP
	// exchange. Pending credentials live only in backend memory.
	PendingTTL = 5 * time.Minute
	// MaxPending bounds concurrent pending credentials so pending-state
	// capacity cannot grow without limit.
	MaxPending = 16
	// MaxPendingAttempts bounds wrong codes per pending token before the
	// token is exhausted.
	MaxPendingAttempts = 5
	// TOTP settings required for authenticator interoperability.
	TOTPIssuer     = "Roaminal"
	TOTPAccount    = "roaminal"
	TOTPPeriod     = 30
	TOTPDigits     = 6
	TOTPSkewSteps  = 1
	TOTPSecretSize = 20
	// EnrollmentCheckTimeout bounds a single live-deletion state check so
	// authentication boundaries never block on an unhealthy filesystem.
	EnrollmentCheckTimeout = time.Second
	// LoginNextStepSetup and LoginNextStepVerify are the two possible
	// post-password stages of the mandatory TOTP login.
	LoginNextStepSetup  = "setup_totp"
	LoginNextStepVerify = "verify_totp"
)

var (
	ErrInvalidChallenge  = errors.New("invalid login challenge")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrLocked            = errors.New("service locked")
	ErrNotFound          = errors.New("auth session not found")
	ErrInvalidPending    = errors.New("invalid or expired pending token")
	ErrEnrollmentStorage = errors.New("auth enrollment storage error")
	ErrAlreadyConfigured = errors.New("TOTP enrollment already configured")
	ErrNotConfigured     = errors.New("TOTP enrollment not configured")
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

// LoginPending is the result of a verified password proof. It never contains
// normal credentials; the pending token is purpose-bound and only usable for
// the single nextStep exchange before it expires.
type LoginPending struct {
	NextStep     string    `json:"nextStep"`
	PendingToken string    `json:"pendingToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// TOTPSetup is the enrollment candidate material for the setup stage. The QR
// image is generated locally by the backend; the URI never leaves for an
// external QR service.
type TOTPSetup struct {
	ProvisioningURI string    `json:"provisioningUri"`
	Secret          string    `json:"secret"`
	QRImage         string    `json:"qrImage"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// TOTPConfirmResult reports a durable enrollment. All credentials issued
// before confirmation are invalid and a fresh password proof plus TOTP is
// required.
type TOTPConfirmResult struct {
	ReauthenticationRequired bool `json:"reauthenticationRequired"`
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
	ID, Salt  string
	ExpiresAt time.Time
}

type enrollmentStatus uint8

const (
	// enrollmentUnconfigured means the private enrollment file is absent:
	// password proof may only reach restricted TOTP setup.
	enrollmentUnconfigured enrollmentStatus = iota
	// enrollmentConfigured means a valid enrollment is cached and every login
	// must complete TOTP verification.
	enrollmentConfigured
	// enrollmentStorageError means the file exists but cannot be trusted.
	// Enrollment and business access stay denied until it is corrected.
	enrollmentStorageError
)

type pendingPurpose uint8

const (
	pendingSetup pendingPurpose = iota + 1
	pendingVerify
)

type setupCandidate struct {
	secret string
	uri    string
	qrPNG  string
}

type pendingEntry struct {
	purpose      pendingPurpose
	expiresAt    time.Time
	attempts     int
	enrollmentID string
	userAgent    string
	candidate    *setupCandidate
}

type Manager struct {
	mu               sync.Mutex
	cfg              config.Config
	authRepository   ports.AuthRepository
	enrollmentRepo   ports.TOTPEnrollmentRepository
	clock            ports.Clock
	ids              ports.IDGenerator
	random           ports.RandomSource
	fingerprint      string
	refresh          map[string]domain.AuthSessionRecord
	access           map[string]accessEntry
	challenges       map[string]challenge
	pending          map[string]pendingEntry
	enrollmentState  enrollmentStatus
	enrollment       *domain.TOTPEnrollmentRecord
	enrollmentChange chan struct{}
	failedAttempts   int
	locked           bool
}

type Dependencies struct {
	Clock      ports.Clock
	IDs        ports.IDGenerator
	Random     ports.RandomSource
	Enrollment ports.TOTPEnrollmentRepository
}

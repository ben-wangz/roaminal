package auth

import (
	"errors"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
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
	ID, Salt  string
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

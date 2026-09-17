package server

import (
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/pquerna/otp/totp"
)

type serverTestClock struct {
	mu sync.Mutex
	at time.Time
}

func newServerTestClock() *serverTestClock {
	return &serverTestClock{at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *serverTestClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *serverTestClock) Since(start time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at.Sub(start)
}
func (c *serverTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// serverTestAuth wraps an enrolled auth manager. Construction completes the
// mandatory initial TOTP enrollment with a controllable clock and advances
// past the confirmation-consumed time step, so tests only exercise the
// verify stage that production browsers see after enrollment.
type serverTestAuth struct {
	*auth.Manager
	clock  *serverTestClock
	secret string
}

func newServerTestAuth(cfg config.Config, store *persistence.Store) (*serverTestAuth, error) {
	repositories := persistence.NewRepositories(store)
	clk := newServerTestClock()
	manager, err := auth.NewWithRepositories(cfg, repositories.Auth, auth.Dependencies{Clock: clk, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: repositories.TOTPEnrollment})
	if err != nil {
		return nil, err
	}
	result := &serverTestAuth{Manager: manager, clock: clk}
	challenge, err := manager.Challenge()
	if err != nil {
		return nil, err
	}
	pending, err := manager.Login(challenge.ChallengeID, auth.Proof(cfg.Password, challenge), "test")
	if err != nil {
		return nil, err
	}
	setup, err := manager.Setup(pending.PendingToken)
	if err != nil {
		return nil, err
	}
	code, err := totp.GenerateCode(setup.Secret, clk.Now())
	if err != nil {
		return nil, err
	}
	if _, err := manager.ConfirmSetup(pending.PendingToken, code); err != nil {
		return nil, err
	}
	result.secret = setup.Secret
	result.clock.Advance(auth.TOTPPeriod * time.Second)
	return result, nil
}

// login performs a full staged login: password proof plus TOTP verification.
func (a *serverTestAuth) login(t *testing.T, password string) auth.Tokens {
	t.Helper()
	challenge, err := a.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := a.Login(challenge.ChallengeID, auth.Proof(password, challenge), "test")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(a.secret, a.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := a.VerifyTOTP(pending.PendingToken, code, "test")
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

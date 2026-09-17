package auth

import (
	"context"
	"errors"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/pquerna/otp/totp"
	"sync"
	"testing"
	"time"
)

type testClock struct {
	mu sync.Mutex
	at time.Time
}

func newTestClock() *testClock { return &testClock{at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.at }

func (c *testClock) Since(t time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at.Sub(t)
}

func (c *testClock) Advance(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.at = c.at.Add(d) }

type fixture struct {
	cfg    config.Config
	store  *persistence.Store
	repos  persistence.Repositories
	clock  *testClock
	mgr    *Manager
	secret string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repos := persistence.NewRepositories(store)
	tc := newTestClock()
	f := &fixture{cfg: config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 30}, store: store, repos: repos, clock: tc}
	f.mgr = f.newManager()
	return f
}

func (f *fixture) newManager() *Manager {
	manager, err := NewWithRepositories(f.cfg, f.repos.Auth, Dependencies{Clock: f.clock, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: f.repos.TOTPEnrollment})
	if err != nil {
		panic(err)
	}
	return manager
}

func (f *fixture) passwordLogin(t *testing.T) LoginPending {
	t.Helper()
	challenge, err := f.mgr.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.mgr.Login(challenge.ChallengeID, Proof(f.cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}
	return pending
}

func (f *fixture) enroll(t *testing.T) string {
	t.Helper()
	pending := f.passwordLogin(t)
	if pending.NextStep != LoginNextStepSetup {
		t.Fatalf("next step = %s, want %s", pending.NextStep, LoginNextStepSetup)
	}
	setup, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	f.secret = setup.Secret
	code, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.ConfirmSetup(pending.PendingToken, code); err != nil {
		t.Fatal(err)
	}
	return setup.Secret
}

func (f *fixture) fullLogin(t *testing.T) Tokens {
	t.Helper()
	pending := f.passwordLogin(t)
	if pending.NextStep != LoginNextStepVerify {
		t.Fatalf("next step = %s, want %s", pending.NextStep, LoginNextStepVerify)
	}
	if f.secret == "" {
		f.secret = f.enrolledSecret(t)
	}
	code, err := totp.GenerateCode(f.secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser")
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func (f *fixture) enrolledSecret(t *testing.T) string {
	t.Helper()
	record, exists, err := f.repos.TOTPEnrollment.LoadEnrollment(context.Background())
	if err != nil || !exists {
		t.Fatalf("load enrollment: exists=%v err=%v", exists, err)
	}
	return record.Secret
}

func TestChallengeLoginAndRefreshRotation(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens := f.fullLogin(t)
	if _, err := f.mgr.Authenticate(tokens.AccessToken); err != nil {
		t.Fatal(err)
	}
	next, err := f.mgr.Refresh(tokens.RefreshToken, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if next.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old access token remains valid: %v", err)
	}
	if err := f.mgr.Logout(next.RefreshToken, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Current(next.SessionID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("logout did not revoke session")
	}
}

func TestPasswordProofAloneNeverIssuesTokens(t *testing.T) {
	f := newFixture(t)
	pending := f.passwordLogin(t)
	if pending.PendingToken == "" || pending.ExpiresAt.IsZero() {
		t.Fatal("login did not return a pending credential")
	}
	if pending.NextStep != LoginNextStepSetup {
		t.Fatalf("fresh install must start with setup, got %s", pending.NextStep)
	}
	if _, err := f.mgr.Authenticate(pending.PendingToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("pending token accepted as bearer credential")
	}
	if _, err := f.mgr.Refresh(pending.PendingToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("pending token accepted as refresh credential")
	}
	if _, err := f.mgr.VerifyTOTP(pending.PendingToken, "123456", "browser"); !errors.Is(err, ErrInvalidPending) {
		t.Fatalf("setup pending token usable for verification: %v", err)
	}
}

func TestLockoutConsumesChallengesAndTOTPFailures(t *testing.T) {
	f := newFixture(t)
	cfg := f.cfg
	cfg.AuthMaxAttempts = 2
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repos := persistence.NewRepositories(store)
	mgr, err := NewWithRepositories(cfg, repos.Auth, Dependencies{Clock: f.clock, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: repos.TOTPEnrollment})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cfg.AuthMaxAttempts; i++ {
		challenge, _ := mgr.Challenge()
		_, err = mgr.Login(challenge.ChallengeID, "00", "")
		if i == cfg.AuthMaxAttempts-1 && !errors.Is(err, ErrLocked) {
			t.Fatalf("expected lockout, got %v", err)
		}
	}
	challenge, _ := mgr.Challenge()
	if _, err := mgr.Login(challenge.ChallengeID, Proof(cfg.Password, challenge), ""); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected locked service, got %v", err)
	}
}

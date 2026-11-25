package db

import (
	"errors"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/pquerna/otp/totp"
)

type authTestClock struct{ now time.Time }

func (c *authTestClock) Now() time.Time                     { return c.now }
func (c *authTestClock) Since(then time.Time) time.Duration { return c.now.Sub(then) }

func TestSQLiteAuthRestartAndEnrollmentFailure(t *testing.T) {
	store, settings := sqliteStore(t)
	cfg := config.Config{Password: "fixture-password", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 10}
	clock := &authTestClock{now: time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)}
	newAuth := func(s *Store) (*auth.Manager, error) {
		return auth.NewWithRepositories(cfg, s, auth.Dependencies{Clock: clock, IDs: identity.UUIDGenerator{}, Random: random.CryptoSource{}, Enrollment: s})
	}
	enrollment := sampleEnrollment(clock.Now().Unix() / 30)
	code, err := totp.GenerateCode(enrollment.Secret, clock.Now())
	requireTransferTestSuccess(t, err)
	requireTransferTestSuccess(t, store.SaveEnrollment(t.Context(), enrollment))
	manager, err := newAuth(store)
	requireTransferTestSuccess(t, err)
	pending := authFixtureLogin(t, manager, cfg.Password, auth.LoginNextStepVerify)
	if _, err := manager.VerifyTOTP(pending, code, "fixture"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("stored TOTP step was accepted: %v", err)
	}
	clock.now = clock.now.Add(30 * time.Second)
	code, err = totp.GenerateCode(enrollment.Secret, clock.Now())
	requireTransferTestSuccess(t, err)
	tokens, err := manager.VerifyTOTP(pending, code, "fixture")
	requireTransferTestSuccess(t, err)
	requireTransferTestSuccess(t, store.Close())

	store, err = OpenExisting(t.Context(), settings)
	requireTransferTestSuccess(t, err)
	t.Cleanup(func() { _ = store.Close() })
	manager, err = newAuth(store)
	requireTransferTestSuccess(t, err)
	if _, err := manager.Authenticate(tokens.AccessToken); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("access token survived runtime restart: %v", err)
	}
	rotated, err := manager.Refresh(tokens.RefreshToken, "fixture")
	if err != nil || rotated.SessionID != tokens.SessionID || rotated.RefreshToken == tokens.RefreshToken {
		t.Fatalf("refresh state not restored and rotated: %v", err)
	}
	if _, err := manager.Refresh(tokens.RefreshToken, "fixture"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("old refresh token accepted: %v", err)
	}
	pending = authFixtureLogin(t, manager, cfg.Password, auth.LoginNextStepVerify)
	if _, err := manager.VerifyTOTP(pending, code, "fixture"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("TOTP replay accepted after database reopen: %v", err)
	}

	clock.now = clock.now.Add(2 * time.Hour)
	manager, err = newAuth(store)
	requireTransferTestSuccess(t, err)
	if authRows, err := store.LoadAuth(t.Context()); err != nil || len(authRows) != 0 {
		t.Fatalf("expired refresh state retained at startup: %v", err)
	}
	if _, err := manager.Refresh(rotated.RefreshToken, "fixture"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expired refresh accepted: %v", err)
	}

	requireTransferTestSuccess(t, store.DB.Model(&enrollmentRow{}).Where("key = ?", primaryKey).Update("secret", "invalid").Error)
	challenge, err := manager.Challenge()
	requireTransferTestSuccess(t, err)
	if _, err := manager.Login(challenge.ChallengeID, auth.Proof(cfg.Password, challenge), "fixture"); !errors.Is(err, auth.ErrEnrollmentStorage) {
		t.Fatalf("corrupt enrollment did not fail closed during login: %v", err)
	}
	if _, err := newAuth(store); err == nil {
		t.Fatal("corrupt enrollment did not fail closed at startup")
	}
}

func authFixtureLogin(t *testing.T, manager *auth.Manager, password, step string) string {
	t.Helper()
	challenge, err := manager.Challenge()
	requireTransferTestSuccess(t, err)
	pending, err := manager.Login(challenge.ChallengeID, auth.Proof(password, challenge), "fixture")
	if err != nil || pending.NextStep != step {
		t.Fatalf("unexpected authentication step: %v", err)
	}
	return pending.PendingToken
}

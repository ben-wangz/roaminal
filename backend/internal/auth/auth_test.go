package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

func testConfig(t *testing.T) (config.Config, *persistence.Store) {
	t.Helper()
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 2}, store
}

func newTestManager(cfg config.Config, store *persistence.Store) (*Manager, error) {
	repositories := persistence.NewRepositories(store)
	return NewWithRepositories(cfg, repositories.Auth, Dependencies{Clock: clock.System{}, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}})
}

func TestChallengeLoginAndRefreshRotation(t *testing.T) {
	cfg, store := testConfig(t)
	manager, err := newTestManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := manager.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := manager.Login(challenge.ChallengeID, Proof(cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(tokens.AccessToken); err != nil {
		t.Fatal(err)
	}
	next, err := manager.Refresh(tokens.RefreshToken, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if next.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	if _, err := manager.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old access token remains valid: %v", err)
	}
	if err := manager.Logout(next.RefreshToken, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Current(next.SessionID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("logout did not revoke session")
	}
}

func TestLockoutConsumesChallenges(t *testing.T) {
	cfg, store := testConfig(t)
	manager, err := newTestManager(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cfg.AuthMaxAttempts; i++ {
		challenge, _ := manager.Challenge()
		_, err = manager.Login(challenge.ChallengeID, "00", "")
		if i == cfg.AuthMaxAttempts-1 && !errors.Is(err, ErrLocked) {
			t.Fatalf("expected lockout, got %v", err)
		}
	}
	challenge, _ := manager.Challenge()
	if _, err := manager.Login(challenge.ChallengeID, Proof(cfg.Password, challenge), ""); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected locked service, got %v", err)
	}
}

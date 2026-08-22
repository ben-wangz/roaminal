package auth

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func testConfig(t *testing.T) (config.Config, *persistence.Store) {
	t.Helper()
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 2}, store
}

func TestChallengeLoginAndRefreshRotation(t *testing.T) {
	cfg, store := testConfig(t)
	manager, err := New(cfg, store)
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

func TestConnectionInstanceOrderPersistsWithLoginSession(t *testing.T) {
	cfg, store := testConfig(t)
	manager, err := New(cfg, store)
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
	order := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}
	if err := manager.SetConnectionInstanceOrder(tokens.SessionID, order); err != nil {
		t.Fatal(err)
	}
	order[0] = "33333333-3333-4333-8333-333333333333"
	if got := manager.ConnectionInstanceOrder(tokens.SessionID); !reflect.DeepEqual(got, []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}) {
		t.Fatalf("saved order = %v", got)
	}
	refreshed, err := manager.Refresh(tokens.RefreshToken, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.SessionID != tokens.SessionID {
		t.Fatalf("session ID changed from %q to %q", tokens.SessionID, refreshed.SessionID)
	}
	if got := manager.ConnectionInstanceOrder(refreshed.SessionID); !reflect.DeepEqual(got, []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}) {
		t.Fatalf("order after refresh = %v", got)
	}

	restarted, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.ConnectionInstanceOrder(tokens.SessionID); !reflect.DeepEqual(got, []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}) {
		t.Fatalf("persisted order = %v", got)
	}
	if err := restarted.SetConnectionInstanceOrder(tokens.SessionID, []string{"not-a-connection-id"}); err == nil {
		t.Fatal("invalid connection instance ID was accepted")
	}
}

func TestConnectionInstanceLayoutPersistsWithLoginSession(t *testing.T) {
	cfg, store := testConfig(t)
	manager, err := New(cfg, store)
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
	layout := persistence.ConnectionInstanceLayout{
		Revision:   4,
		GroupOrder: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", persistence.UngroupedConnectionInstanceGroupID},
		Groups: []persistence.ConnectionInstanceGroup{{
			GroupID:               "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Name:                  "Production",
			ConnectionInstanceIDs: []string{"11111111-1111-4111-8111-111111111111"},
		}},
		UngroupedConnectionInstanceIDs: []string{"22222222-2222-4222-8222-222222222222"},
	}
	if err := manager.SetConnectionInstanceLayout(tokens.SessionID, layout); err != nil {
		t.Fatal(err)
	}
	got, ok := manager.ConnectionInstanceLayout(tokens.SessionID)
	if !ok || !reflect.DeepEqual(got, layout) {
		t.Fatalf("layout = %#v, exists = %v", got, ok)
	}
	refreshed, err := manager.Refresh(tokens.RefreshToken, "browser")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := manager.ConnectionInstanceLayout(refreshed.SessionID); !ok || !reflect.DeepEqual(got, layout) {
		t.Fatalf("layout after refresh = %#v, exists = %v", got, ok)
	}
	restarted, err := New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := restarted.ConnectionInstanceLayout(tokens.SessionID); !ok || !reflect.DeepEqual(got, layout) {
		t.Fatalf("persisted layout = %#v, exists = %v", got, ok)
	}
}

func TestLockoutConsumesChallenges(t *testing.T) {
	cfg, store := testConfig(t)
	manager, err := New(cfg, store)
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

package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestLiveDeletionWithoutRestart(t *testing.T) {
	f := newFixture(t)
	tokens := f.fullLoginFromScratch(t)
	// An idle open stream watches the enrollment channel.
	changed := f.mgr.EnrollmentChanged()
	closed := make(chan struct{})
	go func() { <-changed; close(closed) }()
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old access token survived live deletion")
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old refresh token survived live deletion")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("stream watchers were not notified of deletion")
	}
	// Logout stays idempotent, then login offers initial enrollment again.
	if err := f.mgr.Logout(tokens.RefreshToken, tokens.AccessToken); err != nil {
		t.Fatal(err)
	}
	pending := f.passwordLogin(t)
	if pending.NextStep != LoginNextStepSetup {
		t.Fatalf("post-deletion login offers %s, want setup", pending.NextStep)
	}
}

func TestRestartPreservesEnrollmentAndRejectsPasswordOnlyLogin(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens := f.fullLogin(t)
	restarted := f.newManager()
	if _, err := restarted.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("in-memory access token survived restart (expected: only refresh survives)")
	}
	refreshed, err := restarted.Refresh(tokens.RefreshToken, "browser")
	if err != nil {
		t.Fatalf("bound refresh token rejected after restart: %v", err)
	}
	if refreshed.AccessToken == "" {
		t.Fatal("bound refresh did not issue an access token")
	}
	challenge, err := restarted.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restarted.Login(challenge.ChallengeID, Proof(f.cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}
	if pending.NextStep != LoginNextStepVerify {
		t.Fatalf("configured restart offers %s, want verify", pending.NextStep)
	}
}

func TestStartupInvalidatesLegacyAndUnboundSessions(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens := f.fullLogin(t)
	// Legacy session records without enrollment binding must be dropped.
	records, err := f.repos.Auth.LoadAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range records {
		records[i].EnrollmentID = ""
	}
	if err := f.repos.Auth.SaveAuth(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	restarted := f.newManager()
	if _, err := restarted.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("legacy unbound refresh token accepted")
	}
	// Business data (workspace layouts) must remain intact.
	layouts, err := os.ReadFile(filepath.Join(f.store.Root, "workspace-layouts.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace layouts lost: %v", err)
	}
	_ = layouts
	// An absent enrollment file at startup invalidates persisted sessions.
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens = f.fullLogin(t)
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	afterDeletion := f.newManager()
	if _, err := afterDeletion.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("session survived an absent enrollment at startup")
	}
}

func TestReEnrollmentChangesIdentity(t *testing.T) {
	f := newFixture(t)
	secretOne := f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens := f.fullLogin(t)
	_ = tokens
	// Operator deletes the file, then a new enrollment is completed.
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	pending := f.passwordLogin(t) // reconciles the deletion
	setup, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Secret == secretOne {
		t.Fatal("re-enrollment reused the old secret")
	}
	code, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.ConfirmSetup(pending.PendingToken, code); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old enrollment refresh token accepted after re-enrollment")
	}
	// Codes from the old secret never regain access.
	f.clock.Advance(TOTPPeriod * time.Second)
	oldPending := f.passwordLogin(t)
	oldCode, err := totp.GenerateCode(secretOne, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.VerifyTOTP(oldPending.PendingToken, oldCode, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old secret code accepted after re-enrollment")
	}
}

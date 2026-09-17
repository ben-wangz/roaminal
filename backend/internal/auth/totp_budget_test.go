package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/pquerna/otp/totp"
)

func TestPasswordSuccessDoesNotResetTOTPFailureBudget(t *testing.T) {
	f := newFixture(t)
	cfg := f.cfg
	cfg.AuthMaxAttempts = 3
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repos := persistence.NewRepositories(store)
	mgr, err := NewWithRepositories(cfg, repos.Auth, Dependencies{Clock: f.clock, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: repos.TOTPEnrollment})
	if err != nil {
		t.Fatal(err)
	}
	f.repos = repos
	f.store = store
	f.cfg = cfg
	f.mgr = mgr
	secret := f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	// Two wrong TOTP codes, each after a fresh valid password proof.
	for i := 0; i < 2; i++ {
		challenge, _ := mgr.Challenge()
		pending, err := mgr.Login(challenge.ChallengeID, Proof(cfg.Password, challenge), "browser")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mgr.VerifyTOTP(pending.PendingToken, "000000", "browser"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("wrong code accepted: %v", err)
		}
	}
	// Budget now sits at 2 of 3. A valid password proof plus a third wrong
	// code must lock the service.
	challenge, _ := mgr.Challenge()
	pending, err := mgr.Login(challenge.ChallengeID, Proof(cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.VerifyTOTP(pending.PendingToken, "000000", "browser"); !errors.Is(err, ErrLocked) {
		t.Fatalf("fresh pending tokens bypassed the failure budget: %v", err)
	}
	// A correct code no longer helps while locked.
	challenge, _ = mgr.Challenge()
	pending, err = mgr.Login(challenge.ChallengeID, Proof(cfg.Password, challenge), "browser")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("expected locked service, got %v", err)
	}
	if pending.PendingToken != "" {
		t.Fatal("locked service issued a pending credential")
	}
	code, err := totp.GenerateCode(secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.VerifyTOTP("", code, "browser"); !errors.Is(err, ErrLocked) {
		t.Fatal("locked service still verifiable")
	}
}

func TestPendingTokenExpiryAndAttemptExhaustion(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	pending := f.passwordLogin(t)
	for i := 0; i < MaxPendingAttempts; i++ {
		_, err := f.mgr.VerifyTOTP(pending.PendingToken, "000000", "browser")
		if i < MaxPendingAttempts-1 && !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := f.mgr.VerifyTOTP(pending.PendingToken, "000000", "browser"); !errors.Is(err, ErrInvalidPending) {
		t.Fatalf("exhausted token accepted: %v", err)
	}
	f.clock.Advance(PendingTTL + time.Second)
	expired := f.passwordLogin(t)
	f.clock.Advance(PendingTTL + time.Second)
	code, err := totp.GenerateCode(f.secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.VerifyTOTP(expired.PendingToken, code, "browser"); !errors.Is(err, ErrInvalidPending) {
		t.Fatalf("expired pending token accepted: %v", err)
	}
}

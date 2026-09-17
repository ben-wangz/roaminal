package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/pquerna/otp/totp"
)

func TestConcurrentConfirmationsProduceExactlyOneEnrollment(t *testing.T) {
	f := newFixture(t)
	pendingOne := f.passwordLogin(t)
	pendingTwo := f.passwordLogin(t)
	setupOne, err := f.mgr.Setup(pendingOne.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	setupTwo, err := f.mgr.Setup(pendingTwo.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	codeOne, err := totp.GenerateCode(setupOne.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	codeTwo, err := totp.GenerateCode(setupTwo.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i, attempt := range []struct {
		token, code string
	}{{pendingOne.PendingToken, codeOne}, {pendingTwo.PendingToken, codeTwo}} {
		wg.Add(1)
		go func(slot int, token, code string) {
			defer wg.Done()
			_, err := f.mgr.ConfirmSetup(token, code)
			results[slot] = err
		}(i, attempt.token, attempt.code)
	}
	wg.Wait()
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent confirmations succeeded %d times, want exactly 1 (%v)", successes, results)
	}
	if _, err := f.mgr.Setup(pendingTwo.PendingToken); !errors.Is(err, ErrAlreadyConfigured) && !errors.Is(err, ErrInvalidPending) {
		t.Fatal("competing setup credential survived confirmation")
	}
}

func TestInvalidEnrollmentFileFailsClosed(t *testing.T) {
	for name, content := range map[string]string{
		"empty":          "",
		"malformed":      "{",
		"unknown fields": `{"formatVersion":1,"enrollmentId":"x","unexpected":true}`,
		"bad version":    `{"formatVersion":99,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":0}`,
		"bad secret":     `{"formatVersion":1,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"not-base32!!","lastAcceptedTimeStep":0}`,
		"bad identity":   `{"formatVersion":1,"enrollmentId":"nope","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":0}`,
		"bad step":       `{"formatVersion":1,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":-5}`,
	} {
		t.Run(name, func(t *testing.T) {
			store, err := persistence.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(store.Root, "2fa-secret"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			repos := persistence.NewRepositories(store)
			cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 30}
			if _, err := NewWithRepositories(cfg, repos.Auth, Dependencies{Clock: newTestClock(), IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: repos.TOTPEnrollment}); err == nil {
				t.Fatal("startup accepted an invalid enrollment file")
			}
		})
	}
}

func TestSymlinkEnrollmentFileIsRejected(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("symlink permission checks are meaningless as root")
	}
	f := newFixture(t)
	f.enroll(t)
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte(`{"formatVersion":1,"enrollmentId":"11111111-1111-4111-8111-111111111111","secret":"JBSWY3DPEHPK3PXP","lastAcceptedTimeStep":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	// Runtime corruption is detected at the next authentication boundary.
	if _, err := f.mgr.Authenticate("anything"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("storage error still authenticates: %v", err)
	}
	challenge, err := f.mgr.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Login(challenge.ChallengeID, Proof(f.cfg.Password, challenge), "browser"); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("login during storage error = %v, want storage error", err)
	}
	// And a fresh manager refuses to start against the symlinked file.
	if _, err := NewWithRepositories(f.cfg, f.repos.Auth, Dependencies{Clock: f.clock, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: f.repos.TOTPEnrollment}); err == nil {
		t.Fatal("startup accepted a symlinked enrollment file")
	}
}

func TestRuntimeCorruptionDeniesEverythingUntilCorrected(t *testing.T) {
	f := newFixture(t)
	tokens := f.fullLoginFromScratch(t)
	path := filepath.Join(f.store.Root, "2fa-secret")
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("access granted while enrollment storage is broken")
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) && !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("refresh granted while enrollment storage is broken: %v", err)
	}
	challenge, _ := f.mgr.Challenge()
	if _, err := f.mgr.Login(challenge.ChallengeID, "00", ""); !errors.Is(err, ErrUnauthorized) && !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("login during storage error: %v", err)
	}
	// Restoring a valid enrollment with the same identity re-enables access
	// controls; credentials issued before the error stay invalid.
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) && !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("refresh token survived a storage error window: %v", err)
	}
}

func TestEnrollmentWatchDetectsDeletionPeriodically(t *testing.T) {
	f := newFixture(t)
	f.fullLoginFromScratch(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.mgr.StartEnrollmentWatch(ctx, 10*time.Millisecond)
	changed := f.mgr.EnrollmentChanged()
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("periodic watch did not detect deletion")
	}
	cancel()
}

func TestSessionBoundInterfaceGuard(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens := f.fullLogin(t)
	records, err := f.repos.Auth.LoadAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].EnrollmentID == "" {
		t.Fatalf("persisted session missing enrollment binding: %+v", records)
	}
	if !f.mgr.IsSessionActive(tokens.SessionID) {
		t.Fatal("active session reported inactive")
	}
}

func TestExternalEnrollmentMutationInvalidatesCachedCredentials(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	tokens := f.fullLogin(t)
	record, exists, err := f.repos.TOTPEnrollment.LoadEnrollment(context.Background())
	if err != nil || !exists {
		t.Fatalf("load enrollment: exists=%v err=%v", exists, err)
	}
	// A valid file rewrite with the same enrollment ID is still a credential
	// change. The manager must not continue trusting its cached secret.
	record.Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if err := f.repos.TOTPEnrollment.SaveEnrollment(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cached access token survived enrollment mutation: %v", err)
	}
}

func TestConfirmFailsClosedWhenEnrollmentWriteFails(t *testing.T) {
	f := newFixture(t)
	pending := f.passwordLogin(t)
	setup, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	// A read-only state directory still allows the enrollment read but
	// fails the durable write; confirmation must surface the error instead
	// of configuring an enrollment that is not on disk, and login keeps
	// working as unconfigured afterwards.
	if err := os.Chmod(f.store.Root, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(f.store.Root, 0o700) }()
	code, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.ConfirmSetup(pending.PendingToken, code); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("confirm error = %v, want enrollment storage error", err)
	}
	if _, err := os.Stat(filepath.Join(f.store.Root, "2fa-secret")); !os.IsNotExist(err) {
		t.Fatal("failed confirmation left an enrollment artifact")
	}
	fallback := f.passwordLogin(t)
	if fallback.NextStep != LoginNextStepSetup {
		t.Fatalf("post-failure login offers %s, want setup", fallback.NextStep)
	}
}

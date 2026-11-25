package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/pquerna/otp/totp"
)

type faultEnrollmentRead struct {
	ports.TOTPEnrollmentRepository
	readsBeforeFailure int
}

func (r *faultEnrollmentRead) LoadEnrollment(ctx context.Context) (domain.TOTPEnrollmentRecord, bool, error) {
	if r.readsBeforeFailure == 0 {
		return domain.TOTPEnrollmentRecord{}, false, context.DeadlineExceeded
	}
	r.readsBeforeFailure--
	return r.TOTPEnrollmentRepository.LoadEnrollment(ctx)
}

type countAuthWrites struct {
	ports.AuthRepository
	writes int
}

func (r *countAuthWrites) SaveAuth(ctx context.Context, rows []domain.AuthSessionRecord) error {
	r.writes++
	return r.AuthRepository.SaveAuth(ctx, rows)
}

func TestEnrollmentReadOutagePreservesRefreshSessions(t *testing.T) {
	f := newFixture(t)
	tokens := f.fullLoginFromScratch(t)
	before := loadAuthRows(t, f)
	writes := &countAuthWrites{AuthRepository: f.repos.Auth}
	f.mgr.authRepository = writes
	f.mgr.enrollmentRepo = &faultEnrollmentRead{TOTPEnrollmentRepository: f.repos.TOTPEnrollment}
	changed := f.mgr.EnrollmentChanged()

	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("authenticate with unavailable enrollment: %v", err)
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("refresh with unavailable enrollment: %v", err)
	}
	if _, err := f.mgr.Current(tokens.SessionID); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("current with unavailable enrollment: %v", err)
	}
	if sessions, err := f.mgr.List(tokens.SessionID); sessions != nil || !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("list with unavailable enrollment: %v", err)
	}
	if active, err := f.mgr.IsSessionActive(tokens.SessionID); active || !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("unverified session used for delivery or cleanup: active=%v err=%v", active, err)
	}
	if _, err := f.mgr.Refresh("invalid", "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("invalid refresh misreported as unavailable: %v", err)
	}
	assertEnrollmentSignal(t, changed)
	if writes.writes != 0 || !slices.Equal(loadAuthRows(t, f), before) {
		t.Fatal("read outage wrote to the auth store")
	}

	f.mgr.enrollmentRepo = f.repos.TOTPEnrollment
	if active, err := f.mgr.IsSessionActive(tokens.SessionID); !active || err != nil {
		t.Fatalf("unchanged enrollment did not recover: active=%v err=%v", active, err)
	}
	restarted := f.newManager()
	rotated, err := restarted.Refresh(tokens.RefreshToken, "browser")
	if err != nil || rotated.SessionID != tokens.SessionID || rotated.RefreshToken == tokens.RefreshToken {
		t.Fatalf("original refresh could not rotate after restart: %v", err)
	}
	if _, err := restarted.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("rotated credential not rejected: %v", err)
	}
}

func TestVerifyFinalEnrollmentReadFailureDoesNotConsumeCodeOrRevokeSessions(t *testing.T) {
	f := newFixture(t)
	prior := f.fullLoginFromScratch(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	pending := f.passwordLogin(t)
	code, err := totp.GenerateCode(f.secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	before := loadAuthRows(t, f)
	f.mgr.enrollmentRepo = &faultEnrollmentRead{TOTPEnrollmentRepository: f.repos.TOTPEnrollment, readsBeforeFailure: 1}
	changed := f.mgr.EnrollmentChanged()
	if tokens, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser"); tokens != (Tokens{}) || !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("verify with unavailable final read: %v", err)
	}
	assertEnrollmentSignal(t, changed)
	if !slices.Equal(loadAuthRows(t, f), before) {
		t.Fatal("final read outage changed persisted credentials")
	}

	f.mgr.enrollmentRepo = f.repos.TOTPEnrollment
	if _, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser"); err != nil {
		t.Fatalf("failed final read consumed pending credential or code: %v", err)
	}
	if _, err := f.mgr.Refresh(prior.RefreshToken, "browser"); err != nil {
		t.Fatalf("failed final read invalidated established login: %v", err)
	}
	if _, err := f.mgr.VerifyTOTP(f.passwordLogin(t).PendingToken, code, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("successful verification did not consume the step: %v", err)
	}
}

func TestReadRecoveryInvalidatesConfirmedDeletionOrReplacement(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		name := "deletion"
		if replacement {
			name = "same-ID replacement"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			tokens := f.fullLoginFromScratch(t)
			f.mgr.enrollmentRepo = &faultEnrollmentRead{TOTPEnrollmentRepository: f.repos.TOTPEnrollment}
			if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrEnrollmentStorage) {
				t.Fatalf("refresh during read failure: %v", err)
			}
			if replacement {
				record, exists, err := f.repos.TOTPEnrollment.LoadEnrollment(t.Context())
				if err != nil || !exists {
					t.Fatalf("load test enrollment: %v", err)
				}
				record.Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
				if err := f.repos.TOTPEnrollment.SaveEnrollment(t.Context(), record); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
				t.Fatal(err)
			}
			f.mgr.enrollmentRepo = f.repos.TOTPEnrollment
			if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("refresh after confirmed %s: %v", name, err)
			}
			if len(loadAuthRows(t, f)) != 0 {
				t.Fatal("confirmed change did not invalidate the durable session")
			}
		})
	}
}

func TestConfirmedDeletionWithFailedDurableInvalidationDoesNotResumeOldSessions(t *testing.T) {
	f := newFixture(t)
	tokens := f.fullLoginFromScratch(t)
	original, exists, err := f.repos.TOTPEnrollment.LoadEnrollment(t.Context())
	if err != nil || !exists {
		t.Fatalf("load test enrollment: %v", err)
	}
	f.mgr.authRepository = saveAuthFailureRepository{delegate: f.repos.Auth, err: errors.New("invalidation write failed")}
	changed := f.mgr.EnrollmentChanged()
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("refresh survived confirmed deletion: %v", err)
	}
	assertEnrollmentSignal(t, changed)
	if len(loadAuthRows(t, f)) != 1 {
		t.Fatal("fixture did not leave a stale record after the failed write")
	}
	challenge, _ := f.mgr.Challenge()
	if _, err := f.mgr.Login(challenge.ChallengeID, Proof(f.cfg.Password, challenge), "browser"); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("allowed setup before durable invalidation: %v", err)
	}

	// Even restoration of the exact old enrollment is not an unchanged read
	// after a confirmed deletion. The in-memory denial survives further errors.
	if err := f.repos.TOTPEnrollment.SaveEnrollment(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old access token revived on restoration: %v", err)
	}
	f.mgr.authRepository = f.repos.Auth
	f.mgr.enrollmentRepo = &faultEnrollmentRead{TOTPEnrollmentRepository: f.repos.TOTPEnrollment}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old refresh token revived on durable recovery: %v", err)
	}
	if len(loadAuthRows(t, f)) != 0 {
		t.Fatal("confirmed invalidation was not retried after a further read failure")
	}
	f.mgr.enrollmentRepo = f.repos.TOTPEnrollment
	restarted := f.newManager()
	if _, err := restarted.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old refresh token revived on restart: %v", err)
	}
	f.clock.Advance(TOTPPeriod * time.Second)
	f.fullLogin(t)
}

func loadAuthRows(t *testing.T, f *fixture) []domain.AuthSessionRecord {
	t.Helper()
	records, err := f.repos.Auth.LoadAuth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func assertEnrollmentSignal(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	default:
		t.Fatal("stream watchers were not notified")
	}
}

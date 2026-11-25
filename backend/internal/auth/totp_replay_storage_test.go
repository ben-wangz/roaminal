package auth

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/pquerna/otp/totp"
)

type lostEnrollmentWriteResponse struct{ ports.TOTPEnrollmentRepository }

func (r lostEnrollmentWriteResponse) SaveEnrollment(ctx context.Context, record domain.TOTPEnrollmentRecord) error {
	if err := r.TOTPEnrollmentRepository.SaveEnrollment(ctx, record); err != nil {
		return err
	}
	return context.DeadlineExceeded
}

func TestCommittedReplayStepDoesNotRevokeExistingSessions(t *testing.T) {
	f := newFixture(t)
	previous := f.fullLoginFromScratch(t)
	before := loadAuthRows(t, f)
	f.clock.Advance(TOTPPeriod * time.Second)
	pending := f.passwordLogin(t)
	code, err := totp.GenerateCode(f.secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	f.mgr.enrollmentRepo = lostEnrollmentWriteResponse{f.repos.TOTPEnrollment}
	if tokens, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser"); tokens != (Tokens{}) || !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("verification with an ambiguous write: %v", err)
	}
	f.mgr.enrollmentRepo = f.repos.TOTPEnrollment
	if _, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("committed step became reusable after lost response: %v", err)
	}
	if !slices.Equal(loadAuthRows(t, f), before) {
		t.Fatal("a replay checkpoint advance erased existing login sessions")
	}
	if _, err := f.mgr.Refresh(previous.RefreshToken, "browser"); err != nil {
		t.Fatalf("unchanged enrollment did not preserve existing refresh: %v", err)
	}
}

func TestRegressedReplayStepDeniesAccessWithoutWipingSessions(t *testing.T) {
	f := newFixture(t)
	tokens := f.fullLoginFromScratch(t)
	before := loadAuthRows(t, f)
	record, exists, err := f.repos.TOTPEnrollment.LoadEnrollment(t.Context())
	if err != nil || !exists {
		t.Fatalf("load test enrollment: exists=%t, err=%v", exists, err)
	}
	regressed := record
	regressed.LastAcceptedTimeStep--
	if err := f.repos.TOTPEnrollment.SaveEnrollment(t.Context(), regressed); err != nil {
		t.Fatal(err)
	}
	changed := f.mgr.EnrollmentChanged()
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrEnrollmentStorage) {
		t.Fatalf("access against a regressed replay checkpoint: %v", err)
	}
	assertEnrollmentSignal(t, changed)
	if !slices.Equal(loadAuthRows(t, f), before) {
		t.Fatal("inconsistent checkpoint erased existing login sessions")
	}
	if err := f.repos.TOTPEnrollment.SaveEnrollment(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); err != nil {
		t.Fatalf("restored checkpoint did not preserve existing refresh: %v", err)
	}
}

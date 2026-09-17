package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/pquerna/otp/totp"
)

type saveAuthFailureRepository struct {
	delegate ports.AuthRepository
	err      error
}

func (r saveAuthFailureRepository) LoadAuth(ctx context.Context) ([]domain.AuthSessionRecord, error) {
	return r.delegate.LoadAuth(ctx)
}

func (r saveAuthFailureRepository) SaveAuth(context.Context, []domain.AuthSessionRecord) error {
	return r.err
}

func TestIssueRollsBackCredentialsWhenAuthPersistenceFails(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	pending := f.passwordLogin(t)
	code, err := totp.GenerateCode(f.secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	persistErr := errors.New("auth persistence unavailable")
	f.mgr.authRepository = saveAuthFailureRepository{delegate: f.repos.Auth, err: persistErr}
	if _, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser"); !errors.Is(err, persistErr) {
		t.Fatalf("verify error = %v, want persistence error", err)
	}
	if len(f.mgr.refresh) != 0 || len(f.mgr.access) != 0 {
		t.Fatalf("failed issue left in-memory credentials: refresh=%d access=%d", len(f.mgr.refresh), len(f.mgr.access))
	}
	records, err := f.repos.Auth.LoadAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("failed issue persisted %d auth sessions", len(records))
	}
}

func TestRefreshRestoresPreviousCredentialsWhenAuthPersistenceFails(t *testing.T) {
	f := newFixture(t)
	f.fullLoginFromScratch(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	previous := f.fullLogin(t)
	persistErr := errors.New("auth persistence unavailable")
	f.mgr.authRepository = saveAuthFailureRepository{delegate: f.repos.Auth, err: persistErr}
	if _, err := f.mgr.Refresh(previous.RefreshToken, "browser"); !errors.Is(err, persistErr) {
		t.Fatalf("refresh error = %v, want persistence error", err)
	}
	if _, err := f.mgr.Authenticate(previous.AccessToken); err != nil {
		t.Fatalf("previous access token was not restored: %v", err)
	}
	f.mgr.authRepository = f.repos.Auth
	rotated, err := f.mgr.Refresh(previous.RefreshToken, "browser")
	if err != nil {
		t.Fatalf("restored refresh token was not usable: %v", err)
	}
	if rotated.RefreshToken == previous.RefreshToken {
		t.Fatal("restored refresh did not rotate the token")
	}
}

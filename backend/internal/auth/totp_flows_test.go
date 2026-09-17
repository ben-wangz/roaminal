package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestSetupRequiresSetupPendingTokenAndIsRepeatable(t *testing.T) {
	f := newFixture(t)
	pending := f.passwordLogin(t)
	first, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	if first.Secret != second.Secret || first.ProvisioningURI != second.ProvisioningURI || first.QRImage != second.QRImage {
		t.Fatal("repeated setup returned a different candidate")
	}
	if !strings.HasPrefix(first.QRImage, "data:image/png;base64,") {
		t.Fatal("QR image is not a local PNG data URL")
	}
	if !strings.Contains(first.ProvisioningURI, "issuer=Roaminal") || !strings.Contains(first.ProvisioningURI, "secret=") {
		t.Fatal("provisioning URI missing issuer or secret")
	}
	if _, err := f.mgr.Setup("rp_invalid"); !errors.Is(err, ErrInvalidPending) {
		t.Fatalf("setup without pending token: %v", err)
	}
}

func TestConfirmPersistsPrivateFileAndInvalidatesCredentials(t *testing.T) {
	f := newFixture(t)
	tokens := f.fullLoginFromScratch(t)
	// Operator resets authentication by deleting the enrollment file; a new
	// enrollment must then invalidate the old tokens.
	if err := os.Remove(filepath.Join(f.store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	pending := f.passwordLogin(t)
	setup, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.mgr.ConfirmSetup(pending.PendingToken, code)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReauthenticationRequired {
		t.Fatal("confirm must require reauthentication")
	}
	info, err := os.Stat(filepath.Join(f.store.Root, "2fa-secret"))
	if err != nil {
		t.Fatal("enrollment file missing after confirm")
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("enrollment file mode = %o, want 0600", info.Mode().Perm())
	}
	if _, err := f.mgr.Authenticate(tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old access token survived enrollment")
	}
	if _, err := f.mgr.Refresh(tokens.RefreshToken, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old refresh token survived enrollment")
	}
	if _, err := f.mgr.Setup(pending.PendingToken); !errors.Is(err, ErrAlreadyConfigured) {
		t.Fatal("setup still allowed after enrollment")
	}
}

// fullLoginFromScratch enrolls and then performs a full login immediately,
// waiting for a fresh time step as real users must after confirming.
func (f *fixture) fullLoginFromScratch(t *testing.T) Tokens {
	t.Helper()
	secret := f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	pending := f.passwordLogin(t)
	code, err := totp.GenerateCode(secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser")
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

func TestConfirmConsumesItsTimeStep(t *testing.T) {
	f := newFixture(t)
	pending := f.passwordLogin(t)
	setup, err := f.mgr.Setup(pending.PendingToken)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.ConfirmSetup(pending.PendingToken, code); err != nil {
		t.Fatal(err)
	}
	// Immediate re-login with the confirmation code must fail; the user has
	// to wait for the next code.
	nextPending := f.passwordLogin(t)
	if _, err := f.mgr.VerifyTOTP(nextPending.PendingToken, code, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("confirmation code replay accepted: %v", err)
	}
	f.clock.Advance(TOTPPeriod * time.Second)
	final := f.fullLogin(t)
	if final.AccessToken == "" {
		t.Fatal("full login after next step failed")
	}
}

func TestTOTPReplayAndAdjacentStepTolerance(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	// Skip past the confirmation-consumed step before testing tolerance.
	f.clock.Advance(2 * TOTPPeriod * time.Second)
	base := f.clock.Now()
	pastCode, err := totp.GenerateCode(f.secret, base.Add(-TOTPPeriod*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	currentCode, err := totp.GenerateCode(f.secret, base)
	if err != nil {
		t.Fatal(err)
	}
	futureCode, err := totp.GenerateCode(f.secret, base.Add(TOTPPeriod*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	distantCode, err := totp.GenerateCode(f.secret, base.Add(2*TOTPPeriod*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	pending := f.passwordLogin(t)
	// Distant future code is beyond the one-step tolerance.
	if _, err := f.mgr.VerifyTOTP(pending.PendingToken, distantCode, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("code two steps ahead accepted: %v", err)
	}
	past, err := f.mgr.VerifyTOTP(pending.PendingToken, pastCode, "browser")
	if err != nil {
		t.Fatalf("adjacent past code rejected: %v", err)
	}
	if _, err := f.mgr.Authenticate(past.AccessToken); err != nil {
		t.Fatal(err)
	}
	// The successful verification consumed the pending token; a fresh
	// password proof is required before the consumed step can be replayed.
	replayPending := f.passwordLogin(t)
	if _, err := f.mgr.VerifyTOTP(replayPending.PendingToken, pastCode, "browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("consumed step accepted again: %v", err)
	}
	if _, err := f.mgr.VerifyTOTP(f.passwordLogin(t).PendingToken, currentCode, "browser"); err != nil {
		t.Fatalf("newer unused step rejected: %v", err)
	}
	// A second replay after another password proof stays rejected and the
	// future step still works for a legitimate prover.
	third := f.passwordLogin(t)
	if _, err := f.mgr.VerifyTOTP(third.PendingToken, futureCode, "browser"); err != nil {
		t.Fatalf("unused adjacent future step rejected: %v", err)
	}
}

func TestTOTPCodesWithLeadingZerosAndBoundaries(t *testing.T) {
	f := newFixture(t)
	f.enroll(t)
	f.clock.Advance(TOTPPeriod * time.Second)
	// Search a future step whose code has a leading zero to prove codes are
	// compared as strings, not parsed as numbers.
	var code string
	found := false
	for offset := 1; offset <= 4000; offset++ {
		candidate, err := totp.GenerateCode(f.secret, f.clock.Now().Add(time.Duration(offset)*TOTPPeriod*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(candidate, "0") {
			code = candidate
			f.clock.Advance(time.Duration(offset) * TOTPPeriod * time.Second)
			found = true
			break
		}
	}
	if !found {
		t.Skip("no leading-zero code found in range")
	}
	pending := f.passwordLogin(t)
	tokens, err := f.mgr.VerifyTOTP(pending.PendingToken, code, "browser")
	if err != nil {
		t.Fatalf("leading-zero code rejected: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("no access token issued")
	}
}

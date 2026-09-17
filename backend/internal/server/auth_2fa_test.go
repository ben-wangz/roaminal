package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/ben-wangz/roaminal/backend/internal/workspace"
	"github.com/pquerna/otp/totp"
)

type auth2faFixture struct {
	server  *Server
	cfg     config.Config
	store   *persistence.Store
	manager *auth.Manager
	clock   *serverTestClock
}

func newAuth2faFixture(t *testing.T) *auth2faFixture {
	t.Helper()
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositories := persistence.NewRepositories(store)
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 30}
	clock := newServerTestClock()
	manager, err := auth.NewWithRepositories(cfg, repositories.Auth, auth.Dependencies{Clock: clock, IDs: identity.UUIDGenerator{Random: random.CryptoSource{}}, Random: random.CryptoSource{}, Enrollment: repositories.TOTPEnrollment})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Dependencies{Config: cfg, Version: "0.3.0", BootID: "boot", Auth: manager, Workspace: workspace.New(repositories.Workspace), IDs: identity.UUIDGenerator{}})
	return &auth2faFixture{server: server, cfg: cfg, store: store, manager: manager, clock: clock}
}

func (f *auth2faFixture) post(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test"+path, bytes.NewReader(encoded))
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	return response
}

func (f *auth2faFixture) passwordStage(t *testing.T) auth.LoginPending {
	t.Helper()
	challenge := f.post(t, "/api/v2/auth/challenge", struct{}{})
	if challenge.Code != http.StatusOK {
		t.Fatalf("challenge status=%d", challenge.Code)
	}
	var issued auth.ChallengeResponse
	if err := json.Unmarshal(challenge.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	proof := auth.Proof(f.cfg.Password, auth.ChallengeResponse{
		ChallengeID: issued.ChallengeID, Salt: issued.Salt, ExpiresAt: issued.ExpiresAt, Algorithm: issued.Algorithm,
	})
	login := f.post(t, "/api/v2/auth/login", map[string]string{"challengeId": issued.ChallengeID, "response": proof})
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var pending auth.LoginPending
	if err := json.Unmarshal(login.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	return pending
}

func TestLoginSplitsPasswordProofFromTokenIssuance(t *testing.T) {
	f := newAuth2faFixture(t)
	pending := f.passwordStage(t)
	if pending.NextStep != auth.LoginNextStepSetup {
		t.Fatalf("nextStep=%s, want setup_totp", pending.NextStep)
	}
	if pending.PendingToken == "" || pending.ExpiresAt.Before(f.clock.Now()) {
		t.Fatalf("pending credential missing or already expired: %+v", pending)
	}
	if bytes.Contains([]byte(pending.PendingToken), []byte("ra_")) {
		t.Fatal("login returned a normal access token")
	}
	// The pending token must not open any business route.
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/auth/session", nil)
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Authorization", "Bearer "+pending.PendingToken)
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("business access with pending token: %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("unauthorized auth cache policy=%q, want no-store", got)
	}
}

func TestSetupConfirmAndVerifyContract(t *testing.T) {
	f := newAuth2faFixture(t)
	pending := f.passwordStage(t)
	setupResponse := f.post(t, "/api/v2/auth/2fa/setup", map[string]string{"pendingToken": pending.PendingToken})
	if setupResponse.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", setupResponse.Code, setupResponse.Body.String())
	}
	if got := setupResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("setup cache policy=%q, want no-store", got)
	}
	var setup auth.TOTPSetup
	if err := json.Unmarshal(setupResponse.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if setup.Secret == "" || setup.ProvisioningURI == "" || setup.QRImage == "" {
		t.Fatalf("incomplete setup response: %+v", setup)
	}
	// An invalid confirmation code must never create the enrollment file.
	badConfirm := f.post(t, "/api/v2/auth/2fa/confirm", map[string]string{"pendingToken": pending.PendingToken, "code": "000000"})
	if badConfirm.Code != http.StatusUnauthorized {
		t.Fatalf("invalid confirm status=%d", badConfirm.Code)
	}
	if _, err := os.Stat(filepath.Join(f.store.Root, "2fa-secret")); !os.IsNotExist(err) {
		t.Fatal("invalid confirmation created the enrollment file")
	}
	code, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	confirm := f.post(t, "/api/v2/auth/2fa/confirm", map[string]string{"pendingToken": pending.PendingToken, "code": code})
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirm.Code, confirm.Body.String())
	}
	var confirmed auth.TOTPConfirmResult
	if err := json.Unmarshal(confirm.Body.Bytes(), &confirmed); err != nil || !confirmed.ReauthenticationRequired {
		t.Fatalf("confirm result=%s", confirm.Body.String())
	}
	// Setup stage is closed after enrollment.
	if late := f.post(t, "/api/v2/auth/2fa/setup", map[string]string{"pendingToken": pending.PendingToken}); late.Code != http.StatusConflict && late.Code != http.StatusUnauthorized {
		t.Fatalf("post-enrollment setup status=%d", late.Code)
	}
	// A fresh password proof now owes a verification code.
	f.clock.Advance(auth.TOTPPeriod * time.Second)
	verifyPending := f.passwordStage(t)
	if verifyPending.NextStep != auth.LoginNextStepVerify {
		t.Fatalf("configured login nextStep=%s", verifyPending.NextStep)
	}
	// A verification attempt without a pending token is rejected.
	if direct := f.post(t, "/api/v2/auth/2fa/verify", map[string]string{"code": code}); direct.Code != http.StatusUnauthorized {
		t.Fatalf("verify without pending token status=%d", direct.Code)
	}
	// Setup-purpose pending tokens cannot call verify.
	if cross := f.post(t, "/api/v2/auth/2fa/verify", map[string]string{"pendingToken": pending.PendingToken, "code": code}); cross.Code != http.StatusUnauthorized {
		t.Fatalf("setup token accepted by verify: %d", cross.Code)
	}
	verifyCode, err := totp.GenerateCode(setup.Secret, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	verify := f.post(t, "/api/v2/auth/2fa/verify", map[string]string{"pendingToken": verifyPending.PendingToken, "code": verifyCode})
	if verify.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", verify.Code, verify.Body.String())
	}
	var tokens auth.Tokens
	if err := json.Unmarshal(verify.Body.Bytes(), &tokens); err != nil || tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("verify tokens=%s", verify.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/auth/session", nil)
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("heartbeat with issued tokens: %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("current session cache policy=%q, want no-store", got)
	}
}

func TestSetupRejectsUnknownPendingTokens(t *testing.T) {
	f := newAuth2faFixture(t)
	if response := f.post(t, "/api/v2/auth/2fa/setup", map[string]string{"pendingToken": "rp_unknown"}); response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown pending status=%d", response.Code)
	}
	if response := f.post(t, "/api/v2/auth/2fa/confirm", map[string]string{"pendingToken": "rp_unknown", "code": "123456"}); response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown pending confirm status=%d", response.Code)
	}
}

func TestBrokenEnrollmentStorageSurfaces503(t *testing.T) {
	f := newAuth2faFixture(t)
	pending := f.passwordStage(t)
	setup := f.post(t, "/api/v2/auth/2fa/setup", map[string]string{"pendingToken": pending.PendingToken})
	if setup.Code != http.StatusOK {
		t.Fatalf("setup status=%d", setup.Code)
	}
	if err := os.WriteFile(filepath.Join(f.store.Root, "2fa-secret"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	// While the enrollment file is corrupt, even a valid password proof is
	// denied with a retryable outage instead of inviting enrollment.
	challenge := f.post(t, "/api/v2/auth/challenge", struct{}{})
	if challenge.Code != http.StatusOK {
		t.Fatalf("challenge status=%d", challenge.Code)
	}
	var issued auth.ChallengeResponse
	if err := json.Unmarshal(challenge.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	proof := auth.Proof(f.cfg.Password, issued)
	brokenLogin := f.post(t, "/api/v2/auth/login", map[string]string{"challengeId": issued.ChallengeID, "response": proof})
	if brokenLogin.Code != http.StatusServiceUnavailable {
		t.Fatalf("login during storage error status=%d body=%s", brokenLogin.Code, brokenLogin.Body.String())
	}
	if response := f.post(t, "/api/v2/auth/2fa/setup", map[string]string{"pendingToken": pending.PendingToken}); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("setup during storage error status=%d body=%s", response.Code, response.Body.String())
	}
	if response := f.post(t, "/api/v2/auth/2fa/verify", map[string]string{"pendingToken": pending.PendingToken, "code": "123456"}); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("verify during storage error status=%d body=%s", response.Code, response.Body.String())
	}
}

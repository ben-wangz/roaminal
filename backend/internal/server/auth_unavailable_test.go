package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

type unavailableEnrollment struct {
	ports.TOTPEnrollmentRepository
	readsBeforeFailure int
}

func (r *unavailableEnrollment) LoadEnrollment(ctx context.Context) (domain.TOTPEnrollmentRecord, bool, error) {
	if r.readsBeforeFailure == 0 {
		return domain.TOTPEnrollmentRecord{}, false, context.DeadlineExceeded
	}
	r.readsBeforeFailure--
	return r.TOTPEnrollmentRepository.LoadEnrollment(ctx)
}

func TestAuthUnavailableHTTPContractAndSocketSignal(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := newServerTestClock()
	repos := persistence.NewRepositories(store)
	const secret = "JBSWY3DPEHPK3PXP"
	if err := repos.TOTPEnrollment.SaveEnrollment(t.Context(), domain.TOTPEnrollmentRecord{
		FormatVersion: domain.TOTPFormatVersion, EnrollmentID: "11111111-1111-4111-8111-111111111111", Secret: secret,
	}); err != nil {
		t.Fatal(err)
	}
	enrollment := &unavailableEnrollment{TOTPEnrollmentRepository: repos.TOTPEnrollment, readsBeforeFailure: 10}
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 30}
	manager, err := auth.NewWithRepositories(cfg, repos.Auth, auth.Dependencies{Clock: clock, IDs: identity.UUIDGenerator{}, Random: random.CryptoSource{}, Enrollment: enrollment})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Dependencies{Config: cfg, Auth: manager})
	tokens := (&serverTestAuth{Manager: manager, clock: clock, secret: secret}).login(t, cfg.Password)
	ctx, cancel := context.WithCancel(t.Context())
	stopWatching := server.watchEnrollment(ctx, cancel)
	defer stopWatching()

	assertUnavailable := func(t *testing.T, method, path string, body []byte) {
		t.Helper()
		response := serveNotificationRequest(t, server, method, path, body, tokens.AccessToken)
		var value api.ErrorResponse
		err := json.Unmarshal(response.Body.Bytes(), &value)
		if response.Code != 503 || value.Code != "auth_enrollment_storage_unavailable" || !value.Retryable || err != nil {
			t.Fatalf("%s %s: status=%d body=%s err=%v", method, path, response.Code, response.Body.String(), err)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: unavailable auth response can be cached", path)
		}
	}
	// Authentication middleware passes, but the subsequent auth-session check
	// encounters the read failure. Do not answer with a 401 or empty list.
	for _, path := range []string{"/api/v2/auth/session", "/api/v2/auth/sessions"} {
		enrollment.readsBeforeFailure = 1
		assertUnavailable(t, http.MethodGet, path, nil)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("protected socket watcher was not canceled on the failed read")
	}
	assertUnavailable(t, http.MethodGet, "/api/v2/heartbeat", nil)
	assertUnavailable(t, http.MethodGet, "/ws/v2/browser", nil)
	assertUnavailable(t, http.MethodGet, "/ws/v2/connection-instances/test", nil)
	refreshBody, err := json.Marshal(map[string]string{"refreshToken": tokens.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	assertUnavailable(t, http.MethodPost, "/api/v2/auth/refresh", refreshBody)

	enrollment.readsBeforeFailure = 10
	if response := serveNotificationRequest(t, server, http.MethodGet, "/api/v2/auth/session", nil, tokens.AccessToken); response.Code != http.StatusOK {
		t.Fatalf("unchanged enrollment did not recover: %d %s", response.Code, response.Body.String())
	}
	if err := os.Remove(filepath.Join(store.Root, "2fa-secret")); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method, path string
		body         []byte
	}{
		{http.MethodGet, "/api/v2/auth/session", nil},
		{http.MethodPost, "/api/v2/auth/refresh", refreshBody},
	} {
		response := serveNotificationRequest(t, server, request.method, request.path, request.body, tokens.AccessToken)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("confirmed reset did not deny old credentials: %s %d %s", request.path, response.Code, response.Body.String())
		}
	}
}

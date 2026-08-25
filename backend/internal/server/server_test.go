package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/api"
	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/clientdiag"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/workspace"
)

func TestAPIRoutesTakePrecedenceOverStatic(t *testing.T) {
	server := New(Dependencies{Config: config.Config{}, Version: "0.1.0", BootID: "boot", Static: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})})
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/version", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if strings.Contains(response.Body.String(), "418") {
		t.Fatal("API request was handled by the static adapter")
	}
}

func TestVersionContractAndLegacyRouteBoundary(t *testing.T) {
	server := New(Dependencies{Config: config.Config{}, Version: "0.3.0", BootID: "boot", Static: http.NotFoundHandler()})
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/version", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	var version api.VersionResponse
	if err := json.NewDecoder(response.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	if version.APIVersion != api.Version {
		t.Fatalf("apiVersion = %q, want %q", version.APIVersion, api.Version)
	}
	legacy := httptest.NewRecorder()
	server.Handler().ServeHTTP(legacy, httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/version", nil))
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("legacy route status = %d, want 404", legacy.Code)
	}
}

func TestSameOriginRequiresMatchingHostAndScheme(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest("GET", "http://roaminal.test/api/v2/heartbeat", nil)
	request.Host = "roaminal.test"
	request.Header.Set("Origin", "http://roaminal.test")
	if !server.sameOrigin(request) {
		t.Fatal("expected same-origin request")
	}
	request.Header.Set("Origin", "http://other.test")
	if server.sameOrigin(request) {
		t.Fatal("expected cross-host origin to be rejected")
	}
	request.Header.Set("Origin", "https://roaminal.test")
	if server.sameOrigin(request) {
		t.Fatal("expected mismatched scheme to be rejected")
	}
	request.Header.Set("Origin", "https://roaminal.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	if !server.sameOrigin(request) {
		t.Fatal("expected forwarded HTTPS origin to be accepted")
	}
	request.Header.Set("X-Forwarded-Proto", "wss")
	if !server.sameOrigin(request) {
		t.Fatal("expected forwarded WSS origin to be accepted as HTTPS")
	}
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("X-Forwarded-Proto", "ws")
	if !server.sameOrigin(request) {
		t.Fatal("expected forwarded WS origin to be accepted as HTTP")
	}
	request.Header.Set("Origin", "https://roaminal.test")
	request.Header.Set("X-Forwarded-Proto", "wss, https")
	if !server.sameOrigin(request) {
		t.Fatal("expected first forwarded protocol to determine the origin scheme")
	}
}

func TestAPIRouteAcceptsHTTPSOriginWhenProxyReportsWSS(t *testing.T) {
	server := New(Dependencies{Config: config.Config{}, Version: "0.1.0", BootID: "boot"})
	request := httptest.NewRequest(http.MethodGet, "https://roaminal.test/api/v2/version", nil)
	request.Host = "roaminal.test"
	request.Header.Set("Origin", "https://roaminal.test")
	request.Header.Set("X-Forwarded-Proto", "wss")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestRequestOriginSchemeUsesTLSOverForwardedProtocol(t *testing.T) {
	request := httptest.NewRequest("GET", "https://roaminal.test/api/v2/version", nil)
	request.Header.Set("X-Forwarded-Proto", "http")
	request.TLS = &tls.ConnectionState{}
	if got := requestOriginScheme(request); got != "https" {
		t.Fatalf("requestOriginScheme = %q, want https for TLS request", got)
	}
}

func TestAPIRouteMethodMismatchReturnsStableError(t *testing.T) {
	server := New(Dependencies{Config: config.Config{}, Version: "0.1.0", BootID: "boot"})
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/version", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	var body api.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Code != "method_not_allowed" {
		t.Fatalf("error code = %q, want method_not_allowed", body.Code)
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", response.Header().Get("Allow"))
	}
}

func TestClientDiagnosticsEndpointRequiresAuthAndAcceptsBatch(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 3, ClientDiagnosticsEnabled: true}
	authManager, err := newServerTestAuth(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	sink := clientdiag.New("", "0.2.11", "boot", log.New(&logs, "", 0))
	server := New(Dependencies{Config: cfg, Version: "0.2.11", BootID: "boot", Auth: authManager, Workspace: workspace.New(persistence.NewRepositories(store).Workspace), Diagnostics: sink})
	body := clientdiag.Batch{SchemaVersion: 1, PageID: "11111111-1111-4000-8000-000000000004", Events: []clientdiag.Event{{EventID: "11111111-1111-4000-8000-000000000005", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano), Kind: "console_error", Message: "test"}}}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/client-diagnostics", bytes.NewReader(encoded))
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", response.Code)
	}
	challenge, err := authManager.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := authManager.Login(challenge.ChallengeID, auth.Proof(cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/client-diagnostics", bytes.NewReader(encoded))
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d, want 204: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), "client_diagnostic=") {
		t.Fatalf("missing diagnostic log: %q", logs.String())
	}
}

func TestClientDiagnosticsRouteIsAbsentWhenDisabled(t *testing.T) {
	server := New(Dependencies{Config: config.Config{}})
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/client-diagnostics", bytes.NewReader([]byte(`{}`)))
	request.Header.Set("Origin", "http://roaminal.test")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled route status = %d, want 404", response.Code)
	}
}

func TestClientDiagnosticsDecoderUsesStableErrorsAndSizeLimit(t *testing.T) {
	valid := []byte(`{"schemaVersion":1}`)
	for name, request := range map[string]*http.Request{
		"content type":  httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/client-diagnostics", bytes.NewReader(valid)),
		"unknown field": httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/client-diagnostics", bytes.NewReader([]byte(`{"unknown":true}`))),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "unknown field" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			err := decodeClientDiagnostics(response, request, &clientdiag.Batch{})
			if err == nil || response.Code != http.StatusBadRequest {
				t.Fatalf("decode status=%d err=%v, want 400", response.Code, err)
			}
			var body api.ErrorResponse
			if decodeErr := json.Unmarshal(response.Body.Bytes(), &body); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if body.Code != "invalid_client_diagnostics" {
				t.Fatalf("error code=%q, want invalid_client_diagnostics", body.Code)
			}
		})
	}
	oversized := append(append([]byte{}, valid...), []byte(" \""+strings.Repeat("x", clientdiag.MaxBodyBytes)+"\"")...)
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/v2/client-diagnostics", bytes.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	if err := decodeClientDiagnostics(response, request, &clientdiag.Batch{}); err == nil || response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized decode status=%d err=%v, want 413", response.Code, err)
	}
}

func TestDecodeWebSocketCommandUsesTypedValidation(t *testing.T) {
	command, err := decodeWebSocketCommand([]byte(`{"type":"input","requestId":"request-1","data":"echo \u4f60\u597d\n"}`))
	if err != nil || command.Type != "input" || command.Data != "echo 你好\n" {
		t.Fatalf("command=%+v err=%v", command, err)
	}
	if _, err := decodeWebSocketCommand([]byte(`{"type":"resize","cols":1,"rows":24}`)); err == nil {
		t.Fatal("expected invalid dimensions")
	}
	if _, err := decodeWebSocketCommand([]byte(`{"type":"ping","extra":true}`)); err == nil {
		t.Fatal("expected unknown field rejection")
	}
	invalidObserverInput, err := decodeWebSocketCommand([]byte(`{"type":"input","data":"observer-must-not-write"}`))
	if err == nil || invalidObserverInput.Type != "input" || !isWebSocketControlCommand(invalidObserverInput.Type) {
		t.Fatalf("invalid observer command=%+v err=%v", invalidObserverInput, err)
	}
	if isWebSocketControlCommand("ping") || isWebSocketControlCommand("unknown") {
		t.Fatal("non-control websocket commands must not be classified as control")
	}
}

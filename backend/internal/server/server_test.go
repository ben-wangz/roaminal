package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/clientdiag"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestAPIRoutesTakePrecedenceOverStatic(t *testing.T) {
	server := NewWithStatic(config.Config{}, "0.1.0", "boot", nil, (*connection.Manager)(nil), nil, nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/version", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if strings.Contains(response.Body.String(), "418") {
		t.Fatal("API request was handled by the static adapter")
	}
}

func TestSameOriginRequiresMatchingHostAndScheme(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest("GET", "http://roaminal.test/api/heartbeat", nil)
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
	server := NewWithStatic(config.Config{}, "0.1.0", "boot", nil, (*connection.Manager)(nil), nil, nil, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "https://roaminal.test/api/version", nil)
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
	request := httptest.NewRequest("GET", "https://roaminal.test/api/version", nil)
	request.Header.Set("X-Forwarded-Proto", "http")
	request.TLS = &tls.ConnectionState{}
	if got := requestOriginScheme(request); got != "https" {
		t.Fatalf("requestOriginScheme = %q, want https for TLS request", got)
	}
}

func TestAPIRouteMethodMismatchReturnsStableError(t *testing.T) {
	server := NewWithStatic(config.Config{}, "0.1.0", "boot", nil, (*connection.Manager)(nil), nil, nil, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/version", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body["code"] != "method_not_allowed" {
		t.Fatalf("error code = %q, want method_not_allowed", body["code"])
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", response.Header().Get("Allow"))
	}
}

func TestConnectionInstanceOrderRouteIsNotTreatedAsAnInstanceID(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authManager, err := auth.New(config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour}, store)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithStatic(config.Config{}, "0.1.0", "boot", authManager, (*connection.Manager)(nil), nil, nil, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPut, "http://roaminal.test/api/connection-instances/order", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; route was likely treated as an instance ID", response.Code, http.StatusUnauthorized)
	}
}

func TestConnectionInstanceOrderPrefersSavedIDsAndKeepsUnseenInstances(t *testing.T) {
	instances := []connection.Summary{
		{ID: "first"},
		{ID: "second"},
		{ID: "third"},
	}
	ordered := orderConnectionInstances(instances, []string{"third", "first", "retired"})
	got := make([]string, 0, len(ordered))
	for _, instance := range ordered {
		got = append(got, instance.ID)
	}
	if want := []string{"third", "first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestNormalizeConnectionInstanceOrderDropsRetiredIDsAndAppendsNewInstances(t *testing.T) {
	first := "11111111-1111-4111-8111-111111111111"
	second := "22222222-2222-4222-8222-222222222222"
	third := "33333333-3333-4333-8333-333333333333"
	retired := "44444444-4444-4444-8444-444444444444"
	order, err := normalizeConnectionInstanceOrder([]string{second, retired}, []connection.Summary{{ID: first}, {ID: second}, {ID: third}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{second, first, third}; !reflect.DeepEqual(order, want) {
		t.Fatalf("normalized order = %v, want %v", order, want)
	}
}

func TestClientDiagnosticsEndpointRequiresAuthAndAcceptsBatch(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 3, ClientDiagnosticsEnabled: true}
	authManager, err := auth.New(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	sink := clientdiag.New("", "0.2.11", "boot", log.New(&logs, "", 0))
	server := NewWithSourcesAndDiagnostics(cfg, "0.2.11", "boot", authManager, nil, nil, nil, http.NotFoundHandler(), nil, nil, nil, sink)
	body := clientdiag.Batch{SchemaVersion: 1, PageID: "11111111-1111-4000-8000-000000000004", Events: []clientdiag.Event{{EventID: "11111111-1111-4000-8000-000000000005", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano), Kind: "console_error", Message: "test"}}}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/client-diagnostics", bytes.NewReader(encoded))
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
	request = httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/client-diagnostics", bytes.NewReader(encoded))
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
	server := NewWithSourcesAndDiagnostics(config.Config{}, "0.1.0", "boot", nil, nil, nil, nil, http.NotFoundHandler(), nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/client-diagnostics", bytes.NewReader([]byte(`{}`)))
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
		"content type":  httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/client-diagnostics", bytes.NewReader(valid)),
		"unknown field": httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/client-diagnostics", bytes.NewReader([]byte(`{"unknown":true}`))),
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
			var body map[string]string
			if decodeErr := json.Unmarshal(response.Body.Bytes(), &body); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if body["code"] != "invalid_client_diagnostics" {
				t.Fatalf("error code=%q, want invalid_client_diagnostics", body["code"])
			}
		})
	}
	oversized := append(append([]byte{}, valid...), []byte(" \""+strings.Repeat("x", clientdiag.MaxBodyBytes)+"\"")...)
	request := httptest.NewRequest(http.MethodPost, "http://roaminal.test/api/client-diagnostics", bytes.NewReader(oversized))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	if err := decodeClientDiagnostics(response, request, &clientdiag.Batch{}); err == nil || response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized decode status=%d err=%v, want 413", response.Code, err)
	}
}

func TestValidWSMessageRejectsUnknownFields(t *testing.T) {
	if !validWSMessage("input", map[string]json.RawMessage{"type": rawJSON(`"input"`), "data": rawJSON(`"pwd"`)}) {
		t.Fatal("expected input message")
	}
	if validWSMessage("input", map[string]json.RawMessage{"type": rawJSON(`"input"`), "data": rawJSON(`"pwd"`), "extra": rawJSON(`true`)}) {
		t.Fatal("expected unknown input field to be rejected")
	}
	if validWSMessage("unknown", map[string]json.RawMessage{"type": rawJSON(`"unknown"`)}) {
		t.Fatal("expected unknown message type to be rejected")
	}
}

func rawJSON(value string) []byte { return []byte(value) }

package server

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
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

package server

import (
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

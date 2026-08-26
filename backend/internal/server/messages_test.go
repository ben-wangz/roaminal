package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/auth"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/messages"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestMessageAPIRequiresAuthAndRedactsInternalFields(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 3}
	authManager, err := newServerTestAuth(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	idGenerator := identity.UUIDGenerator{}
	messageService := messages.New(persistence.NewRepositories(store).Messages, idGenerator)
	now := time.Now().UTC()
	if _, _, err := messageService.AppendMessage(domain.MessageDraft{
		Kind: "codex_turn_completed", Severity: "success", AgentType: "codex", PresentationKey: "codex_turn_finished",
		OccurredAt: now, ReceivedAt: now, EndpointKey: "private-endpoint", FallbackLabel: "user@example.test:22 / tmux:roaminal",
		TmuxSessionName: "roaminal", TmuxSessionID: "$0", TmuxSessionCreated: 10, ConnectionInstanceIDs: []string{"instance-1"},
		IdempotencyKey: "private-event\x00codex_turn_completed",
	}); err != nil {
		t.Fatal(err)
	}
	service := New(Dependencies{Config: cfg, Version: "0.3.0", BootID: "boot", Auth: authManager, Messages: messageService})
	unauthorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/messages", nil)
	request.Header.Set("Origin", "http://roaminal.test")
	service.Handler().ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}
	challenge, err := authManager.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := authManager.Login(challenge.ChallengeID, auth.Proof(cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "http://roaminal.test/api/v2/messages?limit=1", nil)
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", response.Code, response.Body.String())
	}
	var page messages.Page
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].Text != "Codex turn finished" || page.Messages[0].Read {
		t.Fatalf("unexpected message page: %+v", page)
	}
	encoded := response.Body.String()
	for _, forbidden := range []string{"private-endpoint", "private-event", "$0", "endpointKey", "idempotencyKey", "tmuxSessionId"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("message response leaked %q: %s", forbidden, encoded)
		}
	}
	readBody := bytes.NewBufferString(`{"readThroughSequence":1}`)
	request = httptest.NewRequest(http.MethodPut, "http://roaminal.test/api/v2/messages/read-state", readBody)
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("read state status = %d: %s", response.Code, response.Body.String())
	}
	invalid := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "http://roaminal.test/api/v2/messages/read-state", strings.NewReader(`{"readThroughSequence":-1}`))
	request.Header.Set("Origin", "http://roaminal.test")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	service.Handler().ServeHTTP(invalid, request)
	var errorBody struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(invalid.Body).Decode(&errorBody)
	if invalid.Code != http.StatusBadRequest || errorBody.Code != "message_read_state_invalid" {
		t.Fatalf("invalid read state status=%d code=%q", invalid.Code, errorBody.Code)
	}
}

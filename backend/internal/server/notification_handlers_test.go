package server

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
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
	"github.com/ben-wangz/roaminal/backend/internal/notifications"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestNotificationSubscriptionAPIIsAuthenticatedAndKeepsKeysPrivate(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositories := persistence.NewRepositories(store)
	cfg := config.Config{Password: "secret", AuthAccessTTL: time.Minute, AuthRefreshTTL: time.Hour, AuthMaxAttempts: 3}
	authManager, err := newServerTestAuth(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	service, err := notifications.New(repositories.PushSubscriptions, identity.UUIDGenerator{}, notifications.Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", Sender: notificationTestSender{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := New(Dependencies{Config: cfg, Auth: authManager, IDs: identity.UUIDGenerator{}, Notifications: service})
	challenge, err := authManager.Challenge()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := authManager.Login(challenge.ChallengeID, auth.Proof(cfg.Password, challenge), "browser")
	if err != nil {
		t.Fatal(err)
	}

	configResponse := serveNotificationRequest(t, server, http.MethodGet, "/api/v2/notifications/config", nil, tokens.AccessToken)
	if configResponse.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", configResponse.Code, configResponse.Body.String())
	}
	var configuration notifications.ConfigResponse
	if err := json.Unmarshal(configResponse.Body.Bytes(), &configuration); err != nil || !configuration.Enabled || configuration.PublicKey != "test-public" {
		t.Fatalf("configuration=%+v err=%v", configuration, err)
	}

	input := serverValidSubscriptionInput(t)
	body, err := json.Marshal(map[string]any{"endpoint": input.Endpoint, "keys": map[string]string{"auth": input.AuthKey, "p256dh": input.P256dhKey}})
	if err != nil {
		t.Fatal(err)
	}
	registered := serveNotificationRequest(t, server, http.MethodPut, "/api/v2/notifications/subscription", body, tokens.AccessToken)
	if registered.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	if strings.Contains(registered.Body.String(), input.Endpoint) || strings.Contains(registered.Body.String(), input.AuthKey) || strings.Contains(registered.Body.String(), input.P256dhKey) {
		t.Fatalf("registration response leaked subscription material: %s", registered.Body.String())
	}
	if records, err := repositories.PushSubscriptions.ListPushSubscriptions(context.Background()); err != nil || len(records) != 1 {
		t.Fatalf("stored records=%+v err=%v", records, err)
	}

	logoutBody := []byte(`{"refreshToken":"` + tokens.RefreshToken + `"}`)
	logout := serveNotificationRequest(t, server, http.MethodPost, "/api/v2/auth/logout", logoutBody, tokens.AccessToken)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", logout.Code, logout.Body.String())
	}
	if records, err := repositories.PushSubscriptions.ListPushSubscriptions(context.Background()); err != nil || len(records) != 0 {
		t.Fatalf("subscriptions after logout=%+v err=%v", records, err)
	}

	unauthorized := serveNotificationRequest(t, server, http.MethodGet, "/api/v2/notifications/config", nil, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d, want 401", unauthorized.Code)
	}
}

func serveNotificationRequest(t *testing.T, service *Server, method, path string, body []byte, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://roaminal.test"+path, bytes.NewReader(body))
	request.Header.Set("Origin", "http://roaminal.test")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	return response
}

func serverValidSubscriptionInput(t *testing.T) notifications.SubscriptionInput {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authKey := make([]byte, 16)
	if _, err := rand.Read(authKey); err != nil {
		t.Fatal(err)
	}
	return notifications.SubscriptionInput{
		Endpoint: "https://push.example.test/send/server-test",
		AuthKey:  base64.RawURLEncoding.EncodeToString(authKey), P256dhKey: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
	}
}

type notificationTestSender struct{}

func (notificationTestSender) Send(context.Context, []byte, domain.PushSubscriptionRecord) (notifications.SendOutcome, error) {
	return notifications.SendOutcome{StatusCode: http.StatusCreated}, nil
}

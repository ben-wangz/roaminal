package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	webpush "github.com/marknefedov/go-webpush/v2"
)

func TestWebPushSenderEncryptsPayloadAndClassifiesExpiredEndpoint(t *testing.T) {
	vapidKeys, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(vapidKeys)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		PublicKey  string `json:"publicKey"`
		PrivateKey string `json:"privateKey"`
	}
	if err := json.Unmarshal(encoded, &config); err != nil {
		t.Fatal(err)
	}
	sender, err := newWebPushSender(config.PublicKey, config.PrivateKey, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	concrete := sender.(*webPushSender)

	var status atomic.Int32
	status.Store(http.StatusCreated)
	pushServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Encoding"); got != "aes128gcm" {
			t.Errorf("content encoding=%q, want aes128gcm", got)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing VAPID authorization")
		}
		w.WriteHeader(int(status.Load()))
	}))
	defer pushServer.Close()
	concrete.client = webpush.NewClient(webpush.Config{HTTPClient: pushServer.Client()})

	input := validSubscriptionInput(t)
	record := domain.PushSubscriptionRecord{Endpoint: pushServer.URL, AuthKey: input.AuthKey, P256dhKey: input.P256dhKey}
	outcome, err := sender.Send(context.Background(), []byte(`{"messageId":"message-1","body":"Codex turn finished"}`), record)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if outcome.StatusCode != http.StatusCreated || outcome.Permanent || outcome.Retryable {
		t.Fatalf("success outcome=%+v", outcome)
	}

	status.Store(http.StatusGone)
	outcome, err = sender.Send(context.Background(), []byte(`{"messageId":"message-2","body":"Codex turn failed"}`), record)
	if err == nil || !outcome.Permanent || outcome.Retryable || outcome.StatusCode != http.StatusGone {
		t.Fatalf("expired outcome=%+v err=%v", outcome, err)
	}
}

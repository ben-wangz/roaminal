package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	webpush "github.com/marknefedov/go-webpush/v2"
)

type webPushSender struct {
	client  *webpush.Client
	keys    *webpush.VAPIDKeys
	subject string
}

func newWebPushSender(publicKey, privateKey, subject string) (Sender, error) {
	data, err := json.Marshal(struct {
		PublicKey  string `json:"publicKey"`
		PrivateKey string `json:"privateKey"`
	}{PublicKey: publicKey, PrivateKey: privateKey})
	if err != nil {
		return nil, err
	}
	var keys webpush.VAPIDKeys
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, err
	}
	if keys.PublicKeyString() != publicKey {
		return nil, errors.New("VAPID public key does not match private key")
	}
	return &webPushSender{
		client:  webpush.NewClient(webpush.Config{HTTPClient: &http.Client{Timeout: 10 * time.Second}}),
		keys:    &keys,
		subject: subject,
	}, nil
}

func (s *webPushSender) Send(ctx context.Context, payload []byte, record domain.PushSubscriptionRecord) (SendOutcome, error) {
	keys, err := webpush.DecodeSubscriptionKeys(record.AuthKey, record.P256dhKey)
	if err != nil {
		return SendOutcome{Permanent: true}, err
	}
	result, err := s.client.Send(ctx, payload, &webpush.Subscription{Endpoint: record.Endpoint, Keys: keys}, webpush.SendOptions{
		Subject: s.subject, TTL: 300, Urgency: webpush.UrgencyHigh, VAPIDKeys: s.keys,
	})
	outcome := SendOutcome{}
	if result != nil {
		outcome.StatusCode = result.StatusCode
		if result.Response != nil {
			_ = result.Response.Body.Close()
		}
	}
	if err == nil {
		return outcome, nil
	}
	var serviceErr *webpush.PushServiceError
	if errors.As(err, &serviceErr) {
		outcome.Permanent = serviceErr.SubscriptionExpired || serviceErr.StatusCode == http.StatusNotFound || serviceErr.StatusCode == http.StatusGone
		outcome.Retryable = serviceErr.Temporary || serviceErr.StatusCode == http.StatusRequestTimeout || serviceErr.StatusCode == http.StatusTooManyRequests
		if serviceErr.StatusCode >= 500 {
			outcome.Retryable = true
		}
		return outcome, err
	}
	// Transport errors and local timeouts are transient. Invalid subscription
	// material was rejected above and is permanent.
	outcome.Retryable = true
	return outcome, err
}

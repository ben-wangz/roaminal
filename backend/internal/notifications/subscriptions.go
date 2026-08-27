package notifications

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"strings"
)

// Prune removes subscriptions belonging to expired authentication sessions.
// It is intentionally best effort at startup: notification availability must
// not prevent the terminal service from starting.
func (s *Service) Prune(ctx context.Context, active func(string) bool) error {
	if s == nil || s.repository == nil || active == nil {
		return nil
	}
	records, err := s.repository.ListPushSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	removed := 0
	for _, record := range records {
		if active(record.AuthenticationSessionID) {
			continue
		}
		if err := s.DeleteByID(ctx, record.ID); err != nil {
			return err
		}
		removed++
	}
	if removed > 0 {
		log.Printf("level=INFO event=web_push_subscriptions_pruned removed=%d", removed)
	}
	return nil
}

func validateSubscription(input SubscriptionInput) error {
	if len([]byte(input.Endpoint)) == 0 || len([]byte(input.Endpoint)) > 4096 {
		return ErrInvalidSubscription
	}
	u, err := url.Parse(input.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return ErrInvalidSubscription
	}
	if _, err := decodeSubscriptionKey(input.AuthKey, 16); err != nil {
		return ErrInvalidSubscription
	}
	if p256dh, err := decodeSubscriptionKey(input.P256dhKey, 65); err != nil {
		return ErrInvalidSubscription
	} else if x, y := elliptic.Unmarshal(elliptic.P256(), p256dh); x == nil || y == nil {
		return ErrInvalidSubscription
	}
	return nil
}

func decodeSubscriptionKey(value string, size int) ([]byte, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "=")
	var (
		decoded []byte
		err     error
	)
	if strings.ContainsAny(value, "+/") {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	} else {
		decoded, err = base64.RawURLEncoding.DecodeString(value)
	}
	if err != nil || len(decoded) != size {
		return nil, ErrInvalidSubscription
	}
	return decoded, nil
}

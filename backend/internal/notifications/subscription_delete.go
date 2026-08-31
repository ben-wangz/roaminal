package notifications

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) Delete(ctx context.Context, authSessionID, subscriptionID string) error {
	if s == nil || s.repository == nil {
		return ErrStoreUnavailable
	}
	authSessionID = strings.TrimSpace(authSessionID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if authSessionID == "" || subscriptionID == "" {
		return ErrInvalidSubscription
	}
	if _, err := s.repository.DeletePushSubscription(ctx, authSessionID, subscriptionID); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

func (s *Service) DeleteAll(ctx context.Context, authSessionID string) error {
	if s == nil || s.repository == nil {
		return ErrStoreUnavailable
	}
	authSessionID = strings.TrimSpace(authSessionID)
	if authSessionID == "" {
		return ErrInvalidSubscription
	}
	if _, err := s.repository.DeletePushSubscriptionsForAuthSession(ctx, authSessionID); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

func (s *Service) DeleteByID(ctx context.Context, subscriptionID string) error {
	if s == nil || s.repository == nil {
		return ErrStoreUnavailable
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ErrInvalidSubscription
	}
	if _, err := s.repository.DeletePushSubscriptionByID(ctx, subscriptionID); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

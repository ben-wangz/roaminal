package persistence

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func (a *repositoryAdapter) ListPushSubscriptions(ctx context.Context) ([]domain.PushSubscriptionRecord, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	a.store.pushMu.Lock()
	defer a.store.pushMu.Unlock()
	file, err := a.store.loadPushSubscriptions()
	if err != nil {
		return nil, a.store.markError(err)
	}
	result := make([]domain.PushSubscriptionRecord, 0, len(file.Subscriptions))
	for _, record := range file.Subscriptions {
		result = append(result, clonePushSubscription(record))
	}
	return result, nil
}

func (a *repositoryAdapter) UpsertPushSubscription(ctx context.Context, subscription domain.PushSubscriptionRecord) (domain.PushSubscriptionRecord, error) {
	if err := checkContext(ctx); err != nil {
		return domain.PushSubscriptionRecord{}, err
	}
	a.store.pushMu.Lock()
	defer a.store.pushMu.Unlock()
	file, err := a.store.loadPushSubscriptions()
	if err != nil {
		return domain.PushSubscriptionRecord{}, a.store.markError(err)
	}
	if err := validatePushSubscriptionFile(pushSubscriptionFile{FormatVersion: StorageSchemaVersion, Subscriptions: []domain.PushSubscriptionRecord{subscription}}); err != nil {
		return domain.PushSubscriptionRecord{}, err
	}
	for index := range file.Subscriptions {
		if file.Subscriptions[index].Endpoint != subscription.Endpoint {
			continue
		}
		if file.Subscriptions[index].AuthenticationSessionID == subscription.AuthenticationSessionID {
			subscription.ID = file.Subscriptions[index].ID
			subscription.CreatedAt = file.Subscriptions[index].CreatedAt
		}
		file.Subscriptions = append(file.Subscriptions[:index], file.Subscriptions[index+1:]...)
		break
	}
	if len(file.Subscriptions) >= maxPushSubscriptions {
		return domain.PushSubscriptionRecord{}, errors.New("push subscription limit reached")
	}
	file.Subscriptions = append(file.Subscriptions, subscription)
	if err := a.store.savePushSubscriptions(file); err != nil {
		return domain.PushSubscriptionRecord{}, err
	}
	return clonePushSubscription(subscription), nil
}

func (a *repositoryAdapter) DeletePushSubscription(ctx context.Context, authSessionID, subscriptionID string) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}
	a.store.pushMu.Lock()
	defer a.store.pushMu.Unlock()
	file, err := a.store.loadPushSubscriptions()
	if err != nil {
		return false, a.store.markError(err)
	}
	for index, record := range file.Subscriptions {
		if record.ID != subscriptionID || record.AuthenticationSessionID != authSessionID {
			continue
		}
		file.Subscriptions = append(file.Subscriptions[:index], file.Subscriptions[index+1:]...)
		if err := a.store.savePushSubscriptions(file); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (a *repositoryAdapter) DeletePushSubscriptionsForAuthSession(ctx context.Context, authSessionID string) (int, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}
	a.store.pushMu.Lock()
	defer a.store.pushMu.Unlock()
	file, err := a.store.loadPushSubscriptions()
	if err != nil {
		return 0, a.store.markError(err)
	}
	retained := file.Subscriptions[:0]
	deleted := 0
	for _, record := range file.Subscriptions {
		if record.AuthenticationSessionID == authSessionID {
			deleted++
			continue
		}
		retained = append(retained, record)
	}
	if deleted == 0 {
		return 0, nil
	}
	file.Subscriptions = append([]domain.PushSubscriptionRecord(nil), retained...)
	if err := a.store.savePushSubscriptions(file); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (a *repositoryAdapter) DeletePushSubscriptionByID(ctx context.Context, subscriptionID string) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}
	a.store.pushMu.Lock()
	defer a.store.pushMu.Unlock()
	file, err := a.store.loadPushSubscriptions()
	if err != nil {
		return false, a.store.markError(err)
	}
	for index, record := range file.Subscriptions {
		if record.ID != subscriptionID {
			continue
		}
		file.Subscriptions = append(file.Subscriptions[:index], file.Subscriptions[index+1:]...)
		if err := a.store.savePushSubscriptions(file); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

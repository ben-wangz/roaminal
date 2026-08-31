package persistence

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func (a *repositoryAdapter) ListNotificationPreferences(ctx context.Context, userKey string) ([]domain.NotificationPreference, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	a.store.notificationPreferencesMu.Lock()
	defer a.store.notificationPreferencesMu.Unlock()
	file, err := a.store.loadNotificationPreferences()
	if err != nil {
		return nil, a.store.markError(err)
	}
	result := make([]domain.NotificationPreference, 0, len(file.Preferences))
	for _, preference := range file.Preferences {
		if preference.UserKey == userKey {
			result = append(result, preference)
		}
	}
	return result, nil
}

func (a *repositoryAdapter) GetNotificationPreference(ctx context.Context, userKey, connectionDefinitionID, tmuxSessionName string) (domain.NotificationPreference, bool, error) {
	if err := checkContext(ctx); err != nil {
		return domain.NotificationPreference{}, false, err
	}
	a.store.notificationPreferencesMu.Lock()
	defer a.store.notificationPreferencesMu.Unlock()
	file, err := a.store.loadNotificationPreferences()
	if err != nil {
		return domain.NotificationPreference{}, false, a.store.markError(err)
	}
	for _, preference := range file.Preferences {
		if preference.UserKey == userKey && preference.ConnectionDefinitionID == connectionDefinitionID && preference.TmuxSessionName == tmuxSessionName {
			return preference, true, nil
		}
	}
	return domain.NotificationPreference{}, false, nil
}

func (a *repositoryAdapter) UpsertNotificationPreference(ctx context.Context, preference domain.NotificationPreference) (domain.NotificationPreference, error) {
	if err := checkContext(ctx); err != nil {
		return domain.NotificationPreference{}, err
	}
	a.store.notificationPreferencesMu.Lock()
	defer a.store.notificationPreferencesMu.Unlock()
	file, err := a.store.loadNotificationPreferences()
	if err != nil {
		return domain.NotificationPreference{}, a.store.markError(err)
	}
	if err := validateNotificationPreference(preference); err != nil {
		return domain.NotificationPreference{}, err
	}
	for index := range file.Preferences {
		current := file.Preferences[index]
		if current.UserKey == preference.UserKey && current.ConnectionDefinitionID == preference.ConnectionDefinitionID && current.TmuxSessionName == preference.TmuxSessionName {
			file.Preferences[index] = preference
			if err := a.store.saveNotificationPreferences(file); err != nil {
				return domain.NotificationPreference{}, err
			}
			return preference, nil
		}
	}
	if len(file.Preferences) >= maxNotificationPreferences {
		return domain.NotificationPreference{}, errors.New("notification preference limit reached")
	}
	file.Preferences = append(file.Preferences, preference)
	if err := a.store.saveNotificationPreferences(file); err != nil {
		return domain.NotificationPreference{}, err
	}
	return preference, nil
}

package notifications

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const maxPreferenceDefinitionIDBytes = 256

func (s *Service) Preferences(ctx context.Context) ([]Preference, error) {
	if s == nil || s.preferences == nil || s.userKey == "" {
		return nil, ErrPreferenceStoreUnavailable
	}
	values, err := s.preferences.ListNotificationPreferences(ctx, s.userKey)
	if err != nil {
		return nil, ErrPreferenceStoreUnavailable
	}
	result := make([]Preference, 0, len(values))
	for _, value := range values {
		result = append(result, publicPreference(value))
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].ConnectionDefinitionID != result[right].ConnectionDefinitionID {
			return result[left].ConnectionDefinitionID < result[right].ConnectionDefinitionID
		}
		return result[left].TmuxSessionName < result[right].TmuxSessionName
	})
	return result, nil
}

func (s *Service) SetPreference(ctx context.Context, input PreferenceInput) (Preference, error) {
	if s == nil || s.preferences == nil || s.userKey == "" {
		return Preference{}, ErrPreferenceStoreUnavailable
	}
	input.ConnectionDefinitionID = strings.TrimSpace(input.ConnectionDefinitionID)
	input.TmuxSessionName = strings.TrimSpace(input.TmuxSessionName)
	if !validPreferenceText(input.ConnectionDefinitionID, maxPreferenceDefinitionIDBytes) || !validTmuxSessionName(input.TmuxSessionName) {
		return Preference{}, ErrInvalidPreference
	}
	_, ok, err := s.preferences.GetNotificationPreference(ctx, s.userKey, input.ConnectionDefinitionID, input.TmuxSessionName)
	if err != nil {
		return Preference{}, ErrPreferenceStoreUnavailable
	}
	if !ok && input.Enabled && !input.RunningToRelax && !input.RunningToError {
		input.RunningToRelax = true
		input.RunningToError = true
	}
	record, err := s.preferences.UpsertNotificationPreference(ctx, domain.NotificationPreference{
		UserKey: s.userKey, ConnectionDefinitionID: input.ConnectionDefinitionID, TmuxSessionName: input.TmuxSessionName,
		Enabled: input.Enabled, RunningToRelax: input.RunningToRelax, RunningToError: input.RunningToError,
		UpdatedAt: s.clock.Now().UTC(),
	})
	if err != nil {
		return Preference{}, ErrPreferenceStoreUnavailable
	}
	return publicPreference(record), nil
}

func publicPreference(value domain.NotificationPreference) Preference {
	return Preference{
		ConnectionDefinitionID: value.ConnectionDefinitionID,
		TmuxSessionName:        value.TmuxSessionName,
		Enabled:                value.Enabled,
		RunningToRelax:         value.RunningToRelax,
		RunningToError:         value.RunningToError,
	}
}

func validPreferenceText(value string, maxBytes int) bool {
	if value == "" || len([]byte(value)) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validTmuxSessionName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if (index == 0 && !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z'))) ||
			(index > 0 && !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-')) {
			return false
		}
	}
	return true
}

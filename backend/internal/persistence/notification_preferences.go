package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

const maxNotificationPreferences = 4096

var notificationTmuxSessionPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type notificationPreferenceFile struct {
	FormatVersion int                             `json:"formatVersion"`
	Preferences   []domain.NotificationPreference `json:"preferences"`
}

func (s *Store) notificationPreferencesPath() string {
	return filepath.Join(s.Root, "notification-preferences.json")
}

func emptyNotificationPreferenceFile() notificationPreferenceFile {
	return notificationPreferenceFile{FormatVersion: StorageSchemaVersion, Preferences: []domain.NotificationPreference{}}
}

func (s *Store) loadNotificationPreferences() (notificationPreferenceFile, error) {
	data, err := os.ReadFile(s.notificationPreferencesPath())
	if errors.Is(err, os.ErrNotExist) {
		return emptyNotificationPreferenceFile(), nil
	}
	if err != nil {
		return notificationPreferenceFile{}, fmt.Errorf("read notification preference repository: %w", err)
	}
	var file notificationPreferenceFile
	if err := decodeStrict(data, &file); err != nil {
		return notificationPreferenceFile{}, fmt.Errorf("decode notification preference repository: %w", err)
	}
	if err := validateNotificationPreferenceFile(file); err != nil {
		return notificationPreferenceFile{}, fmt.Errorf("validate notification preference repository: %w", err)
	}
	return file, nil
}

func (s *Store) saveNotificationPreferences(file notificationPreferenceFile) error {
	file.FormatVersion = StorageSchemaVersion
	if err := validateNotificationPreferenceFile(file); err != nil {
		return s.markError(err)
	}
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(s.notificationPreferencesPath(), append(data, '\n')); err != nil {
		return s.markError(err)
	}
	return nil
}

func (s *Store) initializeNotificationPreferences() error {
	s.notificationPreferencesMu.Lock()
	defer s.notificationPreferencesMu.Unlock()
	if _, err := s.loadNotificationPreferences(); err != nil {
		return s.markError(err)
	}
	return nil
}

func validateNotificationPreferenceFile(file notificationPreferenceFile) error {
	if file.FormatVersion != StorageSchemaVersion {
		return fmt.Errorf("unsupported notification preference schema version %d", file.FormatVersion)
	}
	if len(file.Preferences) > maxNotificationPreferences {
		return errors.New("notification preference retention limit exceeded")
	}
	seen := make(map[string]struct{}, len(file.Preferences))
	for _, preference := range file.Preferences {
		if err := validateNotificationPreference(preference); err != nil {
			return err
		}
		key := preference.UserKey + "\x00" + preference.ConnectionDefinitionID + "\x00" + preference.TmuxSessionName
		if _, exists := seen[key]; exists {
			return errors.New("duplicate notification preference")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateNotificationPreference(preference domain.NotificationPreference) error {
	if !hex64Pattern.MatchString(preference.UserKey) {
		return errors.New("notification preference user key is invalid")
	}
	if !validPreferenceText(preference.ConnectionDefinitionID, 256) {
		return errors.New("notification preference connection definition id is invalid")
	}
	if !notificationTmuxSessionPattern.MatchString(preference.TmuxSessionName) {
		return errors.New("notification preference tmux session name is invalid")
	}
	if preference.UpdatedAt.IsZero() {
		return errors.New("notification preference timestamp is empty")
	}
	return nil
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

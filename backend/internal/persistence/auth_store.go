package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) SaveAuth(file AuthFile) error {
	for _, session := range file.Sessions {
		if err := validateAuthSession(session); err != nil {
			return s.markError(err)
		}
	}
	file.FormatVersion = StorageSchemaVersion
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(filepath.Join(s.Root, "auth-sessions.json"), append(data, '\n')); err != nil {
		return s.markError(err)
	}
	s.degradedMu.Lock()
	s.globalError = false
	s.degradedMu.Unlock()
	return nil
}

func (s *Store) LoadAuth() (AuthFile, error) {
	path := filepath.Join(s.Root, "auth-sessions.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AuthFile{FormatVersion: StorageSchemaVersion, Sessions: []AuthSession{}}, nil
	}
	if err != nil {
		return AuthFile{}, fmt.Errorf("read authentication repository: %w", err)
	}
	var version struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return AuthFile{}, migrationError(path, "decode authentication repository", err)
	}
	if version.FormatVersion == LegacyConnectionFormatVersion {
		return s.migrateLegacyAuth(path, data)
	}
	if version.FormatVersion != StorageSchemaVersion {
		return AuthFile{}, migrationError(path, fmt.Sprintf("unsupported authentication schema version %d", version.FormatVersion), nil)
	}
	var file AuthFile
	if err := decodeStrict(data, &file); err != nil {
		return AuthFile{}, fmt.Errorf("decode authentication repository: %w", err)
	}
	for _, session := range file.Sessions {
		if err := validateAuthSession(session); err != nil {
			return AuthFile{}, fmt.Errorf("validate authentication repository: %w", err)
		}
	}
	return file, nil
}

type legacyAuthSession struct {
	ID                       string                    `json:"id"`
	PasswordFingerprint      string                    `json:"passwordFingerprint"`
	RefreshTokenHash         string                    `json:"refreshTokenHash"`
	CreatedAt                time.Time                 `json:"createdAt"`
	LastSeenAt               time.Time                 `json:"lastSeenAt"`
	RefreshExpiresAt         time.Time                 `json:"refreshExpiresAt"`
	RotatedAt                time.Time                 `json:"rotatedAt"`
	UserAgent                string                    `json:"userAgent"`
	ConnectionInstanceOrder  []string                  `json:"connectionInstanceOrder,omitempty"`
	ConnectionInstanceLayout *ConnectionInstanceLayout `json:"connectionInstanceLayout,omitempty"`
}

type legacyAuthFile struct {
	FormatVersion int                 `json:"formatVersion"`
	Sessions      []legacyAuthSession `json:"sessions"`
}

func (s *Store) migrateLegacyAuth(path string, data []byte) (AuthFile, error) {
	var legacy legacyAuthFile
	if err := decodeStrict(data, &legacy); err != nil {
		return AuthFile{}, migrationError(path, "decode 0.2 authentication repository", err)
	}
	backup, err := s.backupFile(path, "auth-sessions.json")
	if err != nil {
		return AuthFile{}, migrationError(path, "create authentication migration backup", err)
	}
	current := AuthFile{FormatVersion: StorageSchemaVersion, Sessions: make([]AuthSession, 0, len(legacy.Sessions))}
	layouts := make([]workspaceLayoutRecord, 0, len(legacy.Sessions))
	for _, old := range legacy.Sessions {
		converted := AuthSession{ID: old.ID, PasswordFingerprint: old.PasswordFingerprint, RefreshTokenHash: old.RefreshTokenHash, CreatedAt: old.CreatedAt, LastSeenAt: old.LastSeenAt, RefreshExpiresAt: old.RefreshExpiresAt, RotatedAt: old.RotatedAt, UserAgent: old.UserAgent}
		if err := validateAuthSession(converted); err != nil {
			return AuthFile{}, migrationError(backup, "validate 0.2 authentication record", err)
		}
		current.Sessions = append(current.Sessions, converted)
		layout, ok, err := migrateLegacyLayout(old.ConnectionInstanceLayout, old.ConnectionInstanceOrder)
		if err != nil {
			return AuthFile{}, migrationError(backup, "convert 0.2 workspace layout", err)
		}
		if ok {
			layouts = append(layouts, workspaceLayoutRecord{AuthenticationSessionID: old.ID, Layout: layout})
		}
	}
	if err := s.saveWorkspaceLayouts(layouts); err != nil {
		return AuthFile{}, migrationError(backup, "write migrated workspace layouts", err)
	}
	if err := s.SaveAuth(current); err != nil {
		return AuthFile{}, migrationError(backup, "write migrated authentication repository", err)
	}
	if err := s.markMigrationComplete(backup, "auth-sessions.json"); err != nil {
		return AuthFile{}, migrationError(backup, "record authentication migration", err)
	}
	return current, nil
}

func migrateLegacyLayout(layout *ConnectionInstanceLayout, order []string) (ConnectionInstanceLayout, bool, error) {
	if layout != nil {
		if err := ValidateConnectionInstanceLayout(layout); err != nil {
			return ConnectionInstanceLayout{}, false, err
		}
		return *layout, true, nil
	}
	if len(order) == 0 {
		return ConnectionInstanceLayout{}, false, nil
	}
	if err := ValidateConnectionInstanceOrder(order); err != nil {
		return ConnectionInstanceLayout{}, false, err
	}
	return ConnectionInstanceLayout{Revision: 1, GroupOrder: []string{UngroupedConnectionInstanceGroupID}, UngroupedConnectionInstanceIDs: append([]string(nil), order...)}, true, nil
}

func (s *Store) backupFile(path, name string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	dir := filepath.Join(s.Root, "migrations", "0.2-"+stamp)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := ensurePrivateDirectory(filepath.Dir(dir)); err != nil && !errors.Is(err, errWorldPermissions) {
		return "", err
	}
	target := filepath.Join(dir, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(target, content, 0o600); err != nil {
		return "", err
	}
	return target, nil
}

func migrationError(path, message string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%s: migration blocked; backup or repair %s, then restart", message, path)
	}
	return fmt.Errorf("%s: migration blocked for %s: %w; repair the backup and restart", message, path, cause)
}

func (s *Store) quarantine(path, suffix string) error {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return os.Rename(path, path+"."+suffix+"."+stamp)
}

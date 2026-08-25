package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

type workspaceLayoutRecord struct {
	AuthenticationSessionID string                          `json:"authenticationSessionId"`
	Layout                  domain.ConnectionInstanceLayout `json:"layout"`
}

type workspaceLayoutFile struct {
	FormatVersion int                     `json:"formatVersion"`
	Layouts       []workspaceLayoutRecord `json:"layouts"`
}

func (s *Store) workspaceLayoutsPath() string { return filepath.Join(s.Root, "workspace-layouts.json") }

func (s *Store) loadWorkspaceLayouts() (workspaceLayoutFile, error) {
	data, err := os.ReadFile(s.workspaceLayoutsPath())
	if errors.Is(err, os.ErrNotExist) {
		return workspaceLayoutFile{FormatVersion: StorageSchemaVersion, Layouts: []workspaceLayoutRecord{}}, nil
	}
	if err != nil {
		return workspaceLayoutFile{}, fmt.Errorf("read workspace layout repository: %w", err)
	}
	var file workspaceLayoutFile
	if err := decodeStrict(data, &file); err != nil {
		return workspaceLayoutFile{}, fmt.Errorf("decode workspace layout repository: %w", err)
	}
	if file.FormatVersion != StorageSchemaVersion {
		return workspaceLayoutFile{}, fmt.Errorf("unsupported workspace layout schema version %d", file.FormatVersion)
	}
	for _, record := range file.Layouts {
		if err := ValidateConnectionInstanceLayout(&record.Layout); err != nil {
			return workspaceLayoutFile{}, fmt.Errorf("validate workspace layout repository: %w", err)
		}
	}
	return file, nil
}

func (s *Store) saveWorkspaceLayouts(layouts []workspaceLayoutRecord) error {
	file := workspaceLayoutFile{FormatVersion: StorageSchemaVersion, Layouts: layouts}
	for _, record := range file.Layouts {
		if err := ValidateConnectionInstanceLayout(&record.Layout); err != nil {
			return s.markError(err)
		}
	}
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(s.workspaceLayoutsPath(), append(data, '\n')); err != nil {
		return s.markError(err)
	}
	return nil
}

func (s *Store) markMigrationComplete(backup, resource string) error {
	marker := filepath.Join(filepath.Dir(backup), "complete")
	return os.WriteFile(marker, []byte(resource+"\n"), 0o600)
}

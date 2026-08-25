package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

// legacySessionRecord is intentionally decoder-only. It describes the 0.2
// flat session files and is never exposed to application services.
type legacySessionRecord struct {
	FormatVersion                     int             `json:"formatVersion"`
	ID                                string          `json:"id"`
	ConnectionInstanceID              string          `json:"connectionInstanceId"`
	Title                             string          `json:"title"`
	AutomaticTitle                    string          `json:"automaticTitle"`
	TitleOverride                     *string         `json:"titleOverride"`
	InitialCwd                        string          `json:"initialCwd"`
	Cwd                               string          `json:"cwd"`
	Cols                              int             `json:"cols"`
	Rows                              int             `json:"rows"`
	CreatedAt                         time.Time       `json:"createdAt"`
	UpdatedAt                         time.Time       `json:"updatedAt"`
	BackendRuntimeID                  string          `json:"backendRuntimeId"`
	ConnectionDefinitionID            string          `json:"connectionDefinitionId"`
	Type                              string          `json:"type"`
	Purpose                           string          `json:"purpose"`
	SourceHostAlias                   *string         `json:"sourceHostAlias"`
	Lifecycle                         string          `json:"lifecycle"`
	SourceState                       string          `json:"sourceState"`
	ExitCode                          *int            `json:"exitCode"`
	ExitSignal                        *string         `json:"exitSignal"`
	ReuseFromConnectionInstanceID     *string         `json:"reuseFromConnectionInstanceId"`
	ReconnectFromConnectionInstanceID *string         `json:"reconnectFromConnectionInstanceId"`
	RelaunchFromConnectionInstanceID  *string         `json:"relaunchFromConnectionInstanceId"`
	GenerationStatus                  string          `json:"generationStatus"`
	GenerationError                   string          `json:"generationError"`
	GenerationStaging                 string          `json:"generationStaging"`
	TmuxEnabled                       bool            `json:"tmuxEnabled"`
	TmuxSessionName                   string          `json:"tmuxSessionName"`
	TmuxPrefixKey                     string          `json:"tmuxPrefixKey"`
	TmuxPrefixSource                  string          `json:"tmuxPrefixSource"`
	Executions                        json.RawMessage `json:"executions"`
}

func (s *Store) migrateLegacySessions() error {
	sessionsDir := filepath.Join(s.Root, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if errors.Is(err, os.ErrNotExist) || len(entries) == 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy session repository: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	migrationDir := filepath.Join(s.Root, "migrations", "0.2-sessions-"+stamp)
	if err := os.MkdirAll(migrationDir, 0o700); err != nil {
		return fmt.Errorf("prepare session migration: %w", err)
	}
	if err := ensurePrivateDirectory(filepath.Dir(migrationDir)); err != nil && !errors.Is(err, errWorldPermissions) {
		return fmt.Errorf("secure session migration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(migrationDir, "in-progress"), []byte("sessions\n"), 0o600); err != nil {
		return fmt.Errorf("mark session migration: %w", err)
	}
	backupDir := filepath.Join(migrationDir, "sessions")
	if err := os.Rename(sessionsDir, backupDir); err != nil {
		return migrationError(migrationDir, "move 0.2 session repository to backup", err)
	}
	stagingDir := filepath.Join(s.Root, ".connection-instances-migration-"+stamp)
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return migrationError(backupDir, "create connection migration staging area", err)
	}
	staging := &Store{
		Root:           s.Root,
		ConnectionsDir: stagingDir,
		AuditDir:       s.AuditDir,
		DiagnosticsDir: s.DiagnosticsDir,
		Layout:         s.Layout,
		degradedIDs:    make(map[string]struct{}),
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if err := migrateLegacySessionFile(staging, backupDir, entry.Name()); err != nil {
			_ = os.WriteFile(filepath.Join(migrationDir, "failed"), []byte(err.Error()+"\n"), 0o600)
			return migrationError(backupDir, "convert 0.2 session repository", err)
		}
	}
	if err := mergeMigratedConnections(s.ConnectionsDir, stagingDir); err != nil {
		_ = os.WriteFile(filepath.Join(migrationDir, "failed"), []byte(err.Error()+"\n"), 0o600)
		return migrationError(backupDir, "publish migrated connection repository", err)
	}
	_ = os.Remove(filepath.Join(migrationDir, "in-progress"))
	if err := os.WriteFile(filepath.Join(migrationDir, "complete"), []byte("connection-instances\n"), 0o600); err != nil {
		return migrationError(backupDir, "record session migration", err)
	}
	return nil
}

func migrateLegacySessionFile(store *Store, backupDir, name string) error {
	data, err := os.ReadFile(filepath.Join(backupDir, name))
	if err != nil {
		return err
	}
	var record legacySessionRecord
	if err := decodeStrict(data, &record); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	id := record.ID
	if id == "" {
		id = record.ConnectionInstanceID
	}
	if record.AutomaticTitle == "" {
		record.AutomaticTitle = record.Title
	}
	meta := domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: record.BackendRuntimeID, ConnectionDefinitionID: record.ConnectionDefinitionID, Type: record.Type, Purpose: record.Purpose, SourceHostAlias: record.SourceHostAlias, Lifecycle: record.Lifecycle, SourceState: record.SourceState, AutomaticTitle: record.AutomaticTitle, TitleOverride: record.TitleOverride, InitialCwd: record.InitialCwd, Cwd: record.Cwd, Cols: record.Cols, Rows: record.Rows, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, ExitCode: record.ExitCode, ExitSignal: record.ExitSignal, ReuseFromConnectionInstanceID: record.ReuseFromConnectionInstanceID, GenerationStatus: record.GenerationStatus, GenerationError: record.GenerationError, TmuxEnabled: record.TmuxEnabled, TmuxSessionName: record.TmuxSessionName, TmuxPrefixKey: record.TmuxPrefixKey, TmuxPrefixSource: record.TmuxPrefixSource}
	if err := store.SaveConnectionInstance(meta); err != nil {
		return fmt.Errorf("save %s: %w", name, err)
	}
	legacySnapshot := filepath.Join(backupDir, id+".snapshot")
	if snapshot, err := os.ReadFile(legacySnapshot); err == nil {
		if err := os.MkdirAll(filepath.Dir(store.ConnectionSnapshotPath(id)), 0o700); err != nil {
			return err
		}
		if err := store.atomicWrite(store.ConnectionSnapshotPath(id), snapshot); err != nil {
			return fmt.Errorf("copy snapshot for %s: %w", id, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read snapshot for %s: %w", id, err)
	}
	return nil
}

func mergeMigratedConnections(target, staging string) error {
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		if err := os.Remove(target); err != nil {
			return err
		}
		return os.Rename(staging, target)
	}
	defer os.RemoveAll(staging)
	for _, entry := range mustReadDir(staging) {
		destination := filepath.Join(target, entry.Name())
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("connection instance %s already exists", entry.Name())
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(filepath.Join(staging, entry.Name()), destination); err != nil {
			return err
		}
	}
	return nil
}

func mustReadDir(path string) []os.DirEntry {
	entries, _ := os.ReadDir(path)
	return entries
}

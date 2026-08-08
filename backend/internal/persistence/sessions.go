package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) SessionPath(id string) string {
	if s.connectionLayout {
		return filepath.Join(s.ConnectionsDir, id, "metadata.json")
	}
	return filepath.Join(s.SessionsDir, id+".json")
}
func (s *Store) SnapshotPath(id string) string {
	if s.connectionLayout {
		return filepath.Join(s.ConnectionsDir, id, "terminal.snapshot")
	}
	return filepath.Join(s.SessionsDir, id+".snapshot")
}

func (s *Store) SaveSession(meta SessionMeta) error {
	if meta.AutomaticTitle == "" && meta.TitleOverride == nil && meta.Title != "" {
		meta.AutomaticTitle = meta.Title
	}
	meta.SyncEffectiveTitle()
	if err := validateSessionMeta(meta); err != nil {
		return s.markSessionError(meta.ID, err)
	}
	meta.FormatVersion = SessionFormatVersion
	var value any = sessionMetaV2{FormatVersion: SessionFormatVersion, ID: meta.ID, AutomaticTitle: meta.AutomaticTitle, TitleOverride: meta.TitleOverride, InitialCwd: meta.InitialCwd, Cwd: meta.Cwd, Cols: meta.Cols, Rows: meta.Rows, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt, Executions: meta.Executions}
	if s.connectionLayout {
		meta.FormatVersion = ConnectionFormatVersion
		value = connectionMetaV1{FormatVersion: ConnectionFormatVersion, ID: meta.ID, BackendRuntimeID: meta.BackendRuntimeID, ConnectionDefinitionID: meta.ConnectionDefinitionID, Type: meta.Type, Purpose: meta.Purpose, SourceHostAlias: meta.SourceHostAlias, Lifecycle: meta.Lifecycle, SourceState: meta.SourceState, AutomaticTitle: meta.AutomaticTitle, TitleOverride: meta.TitleOverride, InitialCwd: meta.InitialCwd, Cwd: meta.Cwd, Cols: meta.Cols, Rows: meta.Rows, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt, ExitCode: meta.ExitCode, ExitSignal: meta.ExitSignal, HostVerificationAssessment: meta.HostVerificationAssessment, ReuseFromConnectionInstanceID: meta.ReuseFromConnectionInstanceID, ReconnectFromConnectionInstanceID: meta.ReconnectFromConnectionInstanceID, RelaunchFromConnectionInstanceID: meta.RelaunchFromConnectionInstanceID, GenerationStatus: meta.GenerationStatus, GenerationError: meta.GenerationError, GenerationStaging: meta.GenerationStaging}
		if err := os.MkdirAll(filepath.Dir(s.SessionPath(meta.ID)), 0o700); err != nil {
			return s.markSessionError(meta.ID, err)
		}
	}
	data, err := encodeJSON(value)
	if err != nil {
		return s.markSessionError(meta.ID, err)
	}
	if err := s.atomicWrite(s.SessionPath(meta.ID), append(data, '\n')); err != nil {
		return s.markSessionError(meta.ID, err)
	}
	s.clearSessionError(meta.ID)
	return nil
}

func (s *Store) DeleteSession(id string) error {
	if !uuidPattern.MatchString(id) {
		return errors.New("invalid session id")
	}
	if s.connectionLayout {
		if err := os.RemoveAll(filepath.Join(s.ConnectionsDir, id)); err != nil {
			return s.markSessionError(id, err)
		}
	} else {
		for _, path := range []string{s.SessionPath(id), s.SnapshotPath(id)} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return s.markSessionError(id, err)
			}
		}
	}
	s.clearSessionError(id)
	return nil
}

func (s *Store) LoadSession(id string) (SessionMeta, error) {
	if !uuidPattern.MatchString(id) {
		return SessionMeta{}, errors.New("invalid session id")
	}
	data, err := os.ReadFile(s.SessionPath(id))
	if err != nil {
		return SessionMeta{}, err
	}
	meta, err := decodeSessionMeta(data, s.connectionLayout)
	if err != nil || meta.ID != id || validateSessionMeta(meta) != nil {
		_ = s.quarantine(s.SessionPath(id), "corrupt")
		_ = s.quarantine(s.SnapshotPath(id), "corrupt")
		return SessionMeta{}, fmt.Errorf("invalid session metadata %s", id)
	}
	meta.SyncEffectiveTitle()
	return meta, nil
}

type sessionMetaV1 struct {
	FormatVersion int               `json:"formatVersion"`
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	InitialCwd    string            `json:"initialCwd"`
	Cwd           string            `json:"cwd"`
	Cols          int               `json:"cols"`
	Rows          int               `json:"rows"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	Executions    []ExecutionRecord `json:"executions"`
}
type sessionMetaV2 struct {
	FormatVersion  int               `json:"formatVersion"`
	ID             string            `json:"id"`
	AutomaticTitle string            `json:"automaticTitle"`
	TitleOverride  *string           `json:"titleOverride"`
	InitialCwd     string            `json:"initialCwd"`
	Cwd            string            `json:"cwd"`
	Cols           int               `json:"cols"`
	Rows           int               `json:"rows"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	Executions     []ExecutionRecord `json:"executions"`
}

type connectionMetaV1 struct {
	FormatVersion                     int       `json:"formatVersion"`
	ID                                string    `json:"connectionInstanceId"`
	BackendRuntimeID                  string    `json:"backendRuntimeId"`
	ConnectionDefinitionID            string    `json:"connectionDefinitionId"`
	Type                              string    `json:"type"`
	Purpose                           string    `json:"purpose"`
	SourceHostAlias                   *string   `json:"sourceHostAlias"`
	Lifecycle                         string    `json:"lifecycle"`
	SourceState                       string    `json:"sourceState"`
	AutomaticTitle                    string    `json:"automaticTitle"`
	TitleOverride                     *string   `json:"titleOverride"`
	InitialCwd                        string    `json:"initialCwd"`
	Cwd                               string    `json:"cwd"`
	Cols                              int       `json:"cols"`
	Rows                              int       `json:"rows"`
	CreatedAt                         time.Time `json:"createdAt"`
	UpdatedAt                         time.Time `json:"updatedAt"`
	ExitCode                          *int      `json:"exitCode"`
	ExitSignal                        *string   `json:"exitSignal"`
	HostVerificationAssessment        string    `json:"hostVerificationAssessment"`
	ReuseFromConnectionInstanceID     *string   `json:"reuseFromConnectionInstanceId"`
	ReconnectFromConnectionInstanceID *string   `json:"reconnectFromConnectionInstanceId"`
	RelaunchFromConnectionInstanceID  *string   `json:"relaunchFromConnectionInstanceId"`
	GenerationStatus                  string    `json:"generationStatus"`
	GenerationError                   string    `json:"generationError"`
	GenerationStaging                 string    `json:"generationStaging"`
}

func decodeSessionMeta(data []byte, connectionLayout bool) (SessionMeta, error) {
	var version struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return SessionMeta{}, err
	}
	if connectionLayout {
		var current connectionMetaV1
		if err := decodeStrict(data, &current); err != nil {
			return SessionMeta{}, err
		}
		meta := SessionMeta{FormatVersion: ConnectionFormatVersion, ID: current.ID, BackendRuntimeID: current.BackendRuntimeID, ConnectionDefinitionID: current.ConnectionDefinitionID, Type: current.Type, Purpose: current.Purpose, SourceHostAlias: current.SourceHostAlias, Lifecycle: current.Lifecycle, SourceState: current.SourceState, AutomaticTitle: current.AutomaticTitle, TitleOverride: current.TitleOverride, InitialCwd: current.InitialCwd, Cwd: current.Cwd, Cols: current.Cols, Rows: current.Rows, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, ExitCode: current.ExitCode, ExitSignal: current.ExitSignal, HostVerificationAssessment: current.HostVerificationAssessment, ReuseFromConnectionInstanceID: current.ReuseFromConnectionInstanceID, ReconnectFromConnectionInstanceID: current.ReconnectFromConnectionInstanceID, RelaunchFromConnectionInstanceID: current.RelaunchFromConnectionInstanceID, GenerationStatus: current.GenerationStatus, GenerationError: current.GenerationError, GenerationStaging: current.GenerationStaging}
		meta.SyncEffectiveTitle()
		return meta, nil
	}
	switch version.FormatVersion {
	case FormatVersion:
		var legacy sessionMetaV1
		if err := decodeStrict(data, &legacy); err != nil {
			return SessionMeta{}, err
		}
		meta := SessionMeta{FormatVersion: SessionFormatVersion, ID: legacy.ID, AutomaticTitle: legacy.Title, InitialCwd: legacy.InitialCwd, Cwd: legacy.Cwd, Cols: legacy.Cols, Rows: legacy.Rows, CreatedAt: legacy.CreatedAt, UpdatedAt: legacy.UpdatedAt, Executions: legacy.Executions}
		meta.SyncEffectiveTitle()
		return meta, nil
	case SessionFormatVersion:
		var current sessionMetaV2
		if err := decodeStrict(data, &current); err != nil {
			return SessionMeta{}, err
		}
		meta := SessionMeta{FormatVersion: current.FormatVersion, ID: current.ID, AutomaticTitle: current.AutomaticTitle, TitleOverride: current.TitleOverride, InitialCwd: current.InitialCwd, Cwd: current.Cwd, Cols: current.Cols, Rows: current.Rows, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, Executions: current.Executions}
		meta.SyncEffectiveTitle()
		return meta, nil
	default:
		return SessionMeta{}, errors.New("unsupported session format version")
	}
}

func (s *Store) ListSessions() ([]SessionMeta, error) {
	if s.connectionLayout {
		entries, err := os.ReadDir(s.ConnectionsDir)
		if err != nil {
			return nil, err
		}
		result := make([]SessionMeta, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			meta, err := s.LoadSession(entry.Name())
			if err == nil {
				result = append(result, meta)
			}
		}
		for i := 1; i < len(result); i++ {
			for j := i; j > 0 && result[j].CreatedAt.Before(result[j-1].CreatedAt); j-- {
				result[j], result[j-1] = result[j-1], result[j]
			}
		}
		return result, nil
	}
	entries, err := os.ReadDir(s.SessionsDir)
	if err != nil {
		return nil, err
	}
	result := make([]SessionMeta, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		meta, err := s.LoadSession(id)
		if err == nil {
			result = append(result, meta)
		}
	}
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].CreatedAt.Before(result[j-1].CreatedAt); j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result, nil
}

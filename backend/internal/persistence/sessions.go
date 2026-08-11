package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) SessionPath(id string) string {
	return filepath.Join(s.ConnectionsDir, id, "metadata.json")
}
func (s *Store) SnapshotPath(id string) string {
	return filepath.Join(s.ConnectionsDir, id, "terminal.snapshot")
}

func (s *Store) SaveSession(meta SessionMeta) error {
	if meta.AutomaticTitle == "" && meta.TitleOverride == nil && meta.Title != "" {
		meta.AutomaticTitle = meta.Title
	}
	meta.SyncEffectiveTitle()
	if err := validateSessionMeta(meta); err != nil {
		return s.markSessionError(meta.ID, err)
	}
	meta.FormatVersion = ConnectionFormatVersion
	value := connectionMetaV2{FormatVersion: ConnectionFormatVersion, ID: meta.ID, BackendRuntimeID: meta.BackendRuntimeID, ConnectionDefinitionID: meta.ConnectionDefinitionID, Type: meta.Type, Purpose: meta.Purpose, SourceHostAlias: meta.SourceHostAlias, Lifecycle: meta.Lifecycle, SourceState: meta.SourceState, AutomaticTitle: meta.AutomaticTitle, TitleOverride: meta.TitleOverride, InitialCwd: meta.InitialCwd, Cwd: meta.Cwd, Cols: meta.Cols, Rows: meta.Rows, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt, ExitCode: meta.ExitCode, ExitSignal: meta.ExitSignal, HostVerificationAssessment: meta.HostVerificationAssessment, ReuseFromConnectionInstanceID: meta.ReuseFromConnectionInstanceID, ReconnectFromConnectionInstanceID: meta.ReconnectFromConnectionInstanceID, RelaunchFromConnectionInstanceID: meta.RelaunchFromConnectionInstanceID, GenerationStatus: meta.GenerationStatus, GenerationError: meta.GenerationError, GenerationStaging: meta.GenerationStaging, TmuxEnabled: meta.TmuxEnabled, TmuxSessionName: meta.TmuxSessionName, TmuxPrefixKey: meta.TmuxPrefixKey, TmuxPrefixSource: meta.TmuxPrefixSource}
	if err := os.MkdirAll(filepath.Dir(s.SessionPath(meta.ID)), 0o700); err != nil {
		return s.markSessionError(meta.ID, err)
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
	if err := os.RemoveAll(filepath.Join(s.ConnectionsDir, id)); err != nil {
		return s.markSessionError(id, err)
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
	meta, err := decodeSessionMeta(data)
	if err != nil || meta.ID != id || validateSessionMeta(meta) != nil {
		_ = s.quarantine(s.SessionPath(id), "corrupt")
		_ = s.quarantine(s.SnapshotPath(id), "corrupt")
		return SessionMeta{}, fmt.Errorf("invalid session metadata %s", id)
	}
	meta.SyncEffectiveTitle()
	return meta, nil
}

type connectionMetaV1 struct {
	FormatVersion                     int                     `json:"formatVersion"`
	ID                                string                  `json:"connectionInstanceId"`
	BackendRuntimeID                  string                  `json:"backendRuntimeId"`
	ConnectionDefinitionID            string                  `json:"connectionDefinitionId"`
	Type                              string                  `json:"type"`
	Purpose                           string                  `json:"purpose"`
	SourceHostAlias                   *string                 `json:"sourceHostAlias"`
	Lifecycle                         string                  `json:"lifecycle"`
	SourceState                       string                  `json:"sourceState"`
	AutomaticTitle                    string                  `json:"automaticTitle"`
	TitleOverride                     *string                 `json:"titleOverride"`
	InitialCwd                        string                  `json:"initialCwd"`
	Cwd                               string                  `json:"cwd"`
	Cols                              int                     `json:"cols"`
	Rows                              int                     `json:"rows"`
	CreatedAt                         time.Time               `json:"createdAt"`
	UpdatedAt                         time.Time               `json:"updatedAt"`
	ExitCode                          *int                    `json:"exitCode"`
	ExitSignal                        *string                 `json:"exitSignal"`
	HostVerificationAssessment        string                  `json:"hostVerificationAssessment"`
	ReuseFromConnectionInstanceID     *string                 `json:"reuseFromConnectionInstanceId"`
	ReconnectFromConnectionInstanceID *string                 `json:"reconnectFromConnectionInstanceId"`
	RelaunchFromConnectionInstanceID  *string                 `json:"relaunchFromConnectionInstanceId"`
	GenerationStatus                  string                  `json:"generationStatus"`
	GenerationError                   string                  `json:"generationError"`
	GenerationStaging                 string                  `json:"generationStaging"`
	TmuxEnabled                       bool                    `json:"tmuxEnabled"`
	TmuxSessionName                   string                  `json:"tmuxSessionName"`
	TmuxPrefixKey                     string                  `json:"tmuxPrefixKey"`
	TmuxPrefixSource                  string                  `json:"tmuxPrefixSource"`
	Executions                        []legacyExecutionRecord `json:"executions"`
}

type legacyExecutionRecord struct {
	Command     string    `json:"command"`
	ExitCode    *int      `json:"exitCode"`
	Input       string    `json:"input"`
	Output      string    `json:"output"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DurationMs  int64     `json:"durationMs"`
	Truncated   bool      `json:"truncated"`
}

type connectionMetaV2 struct {
	FormatVersion                     int                     `json:"formatVersion"`
	ID                                string                  `json:"connectionInstanceId"`
	BackendRuntimeID                  string                  `json:"backendRuntimeId"`
	ConnectionDefinitionID            string                  `json:"connectionDefinitionId"`
	Type                              string                  `json:"type"`
	Purpose                           string                  `json:"purpose"`
	SourceHostAlias                   *string                 `json:"sourceHostAlias"`
	Lifecycle                         string                  `json:"lifecycle"`
	SourceState                       string                  `json:"sourceState"`
	AutomaticTitle                    string                  `json:"automaticTitle"`
	TitleOverride                     *string                 `json:"titleOverride"`
	InitialCwd                        string                  `json:"initialCwd"`
	Cwd                               string                  `json:"cwd"`
	Cols                              int                     `json:"cols"`
	Rows                              int                     `json:"rows"`
	CreatedAt                         time.Time               `json:"createdAt"`
	UpdatedAt                         time.Time               `json:"updatedAt"`
	ExitCode                          *int                    `json:"exitCode"`
	ExitSignal                        *string                 `json:"exitSignal"`
	HostVerificationAssessment        string                  `json:"hostVerificationAssessment"`
	ReuseFromConnectionInstanceID     *string                 `json:"reuseFromConnectionInstanceId"`
	ReconnectFromConnectionInstanceID *string                 `json:"reconnectFromConnectionInstanceId"`
	RelaunchFromConnectionInstanceID  *string                 `json:"relaunchFromConnectionInstanceId"`
	GenerationStatus                  string                  `json:"generationStatus"`
	GenerationError                   string                  `json:"generationError"`
	GenerationStaging                 string                  `json:"generationStaging"`
	TmuxEnabled                       bool                    `json:"tmuxEnabled"`
	TmuxSessionName                   string                  `json:"tmuxSessionName"`
	TmuxPrefixKey                     string                  `json:"tmuxPrefixKey"`
	TmuxPrefixSource                  string                  `json:"tmuxPrefixSource"`
	Executions                        []legacyExecutionRecord `json:"executions,omitempty"`
}

func decodeSessionMeta(data []byte) (SessionMeta, error) {
	var version struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return SessionMeta{}, err
	}
	switch version.FormatVersion {
	case LegacyConnectionFormatVersion:
		var current connectionMetaV1
		if err := decodeStrict(data, &current); err != nil {
			return SessionMeta{}, err
		}
		meta := SessionMeta{FormatVersion: ConnectionFormatVersion, ID: current.ID, BackendRuntimeID: current.BackendRuntimeID, ConnectionDefinitionID: current.ConnectionDefinitionID, Type: current.Type, Purpose: current.Purpose, SourceHostAlias: current.SourceHostAlias, Lifecycle: current.Lifecycle, SourceState: current.SourceState, AutomaticTitle: current.AutomaticTitle, TitleOverride: current.TitleOverride, InitialCwd: current.InitialCwd, Cwd: current.Cwd, Cols: current.Cols, Rows: current.Rows, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, ExitCode: current.ExitCode, ExitSignal: current.ExitSignal, HostVerificationAssessment: current.HostVerificationAssessment, ReuseFromConnectionInstanceID: current.ReuseFromConnectionInstanceID, ReconnectFromConnectionInstanceID: current.ReconnectFromConnectionInstanceID, RelaunchFromConnectionInstanceID: current.RelaunchFromConnectionInstanceID, GenerationStatus: current.GenerationStatus, GenerationError: current.GenerationError, GenerationStaging: current.GenerationStaging, TmuxEnabled: current.TmuxEnabled, TmuxSessionName: current.TmuxSessionName, TmuxPrefixKey: current.TmuxPrefixKey, TmuxPrefixSource: current.TmuxPrefixSource}
		meta.SyncEffectiveTitle()
		return meta, nil
	case ConnectionFormatVersion:
		var current connectionMetaV2
		if err := decodeStrict(data, &current); err != nil {
			return SessionMeta{}, err
		}
		meta := SessionMeta{FormatVersion: ConnectionFormatVersion, ID: current.ID, BackendRuntimeID: current.BackendRuntimeID, ConnectionDefinitionID: current.ConnectionDefinitionID, Type: current.Type, Purpose: current.Purpose, SourceHostAlias: current.SourceHostAlias, Lifecycle: current.Lifecycle, SourceState: current.SourceState, AutomaticTitle: current.AutomaticTitle, TitleOverride: current.TitleOverride, InitialCwd: current.InitialCwd, Cwd: current.Cwd, Cols: current.Cols, Rows: current.Rows, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, ExitCode: current.ExitCode, ExitSignal: current.ExitSignal, HostVerificationAssessment: current.HostVerificationAssessment, ReuseFromConnectionInstanceID: current.ReuseFromConnectionInstanceID, ReconnectFromConnectionInstanceID: current.ReconnectFromConnectionInstanceID, RelaunchFromConnectionInstanceID: current.RelaunchFromConnectionInstanceID, GenerationStatus: current.GenerationStatus, GenerationError: current.GenerationError, GenerationStaging: current.GenerationStaging, TmuxEnabled: current.TmuxEnabled, TmuxSessionName: current.TmuxSessionName, TmuxPrefixKey: current.TmuxPrefixKey, TmuxPrefixSource: current.TmuxPrefixSource}
		meta.SyncEffectiveTitle()
		return meta, nil
	default:
		return SessionMeta{}, errors.New("unsupported session format version")
	}
}

func (s *Store) ListSessions() ([]SessionMeta, error) {
	entries, err := os.ReadDir(s.ConnectionsDir)
	if err != nil {
		return nil, err
	}
	result := make([]SessionMeta, 0)
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

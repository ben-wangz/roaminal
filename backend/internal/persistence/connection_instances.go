package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) ConnectionInstancePath(id string) string {
	return filepath.Join(s.ConnectionsDir, id, "metadata.json")
}
func (s *Store) ConnectionSnapshotPath(id string) string {
	return filepath.Join(s.ConnectionsDir, id, "terminal.snapshot")
}

func (s *Store) SaveConnectionInstance(meta ConnectionInstanceMeta) error {
	if meta.AutomaticTitle == "" && meta.TitleOverride == nil && meta.Title != "" {
		meta.AutomaticTitle = meta.Title
	}
	meta.SyncEffectiveTitle()
	if err := validateConnectionInstanceMeta(meta); err != nil {
		return s.markConnectionInstanceError(meta.ID, err)
	}
	meta.FormatVersion = ConnectionFormatVersion
	value := connectionMetaV2{FormatVersion: ConnectionFormatVersion, ID: meta.ID, BackendRuntimeID: meta.BackendRuntimeID, ConnectionDefinitionID: meta.ConnectionDefinitionID, Type: meta.Type, Purpose: meta.Purpose, SourceHostAlias: meta.SourceHostAlias, Lifecycle: meta.Lifecycle, SourceState: meta.SourceState, AutomaticTitle: meta.AutomaticTitle, TitleOverride: meta.TitleOverride, InitialCwd: meta.InitialCwd, Cwd: meta.Cwd, Cols: meta.Cols, Rows: meta.Rows, CreatedAt: meta.CreatedAt, UpdatedAt: meta.UpdatedAt, ExitCode: meta.ExitCode, ExitSignal: meta.ExitSignal, ReuseFromConnectionInstanceID: meta.ReuseFromConnectionInstanceID, GenerationStatus: meta.GenerationStatus, GenerationError: meta.GenerationError, TmuxEnabled: meta.TmuxEnabled, TmuxSessionName: meta.TmuxSessionName, TmuxPrefixKey: meta.TmuxPrefixKey, TmuxPrefixSource: meta.TmuxPrefixSource}
	if err := os.MkdirAll(filepath.Dir(s.ConnectionInstancePath(meta.ID)), 0o700); err != nil {
		return s.markConnectionInstanceError(meta.ID, err)
	}
	data, err := encodeJSON(value)
	if err != nil {
		return s.markConnectionInstanceError(meta.ID, err)
	}
	if err := s.atomicWrite(s.ConnectionInstancePath(meta.ID), append(data, '\n')); err != nil {
		return s.markConnectionInstanceError(meta.ID, err)
	}
	s.clearConnectionInstanceError(meta.ID)
	return nil
}

func (s *Store) DeleteConnectionInstance(id string) error {
	if !uuidPattern.MatchString(id) {
		return errors.New("invalid connection instance id")
	}
	if err := os.RemoveAll(filepath.Join(s.ConnectionsDir, id)); err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	s.clearConnectionInstanceError(id)
	return nil
}

func (s *Store) LoadConnectionInstance(id string) (ConnectionInstanceMeta, error) {
	if !uuidPattern.MatchString(id) {
		return ConnectionInstanceMeta{}, errors.New("invalid connection instance id")
	}
	data, err := os.ReadFile(s.ConnectionInstancePath(id))
	if err != nil {
		return ConnectionInstanceMeta{}, err
	}
	meta, err := decodeConnectionInstanceMeta(data)
	if err != nil || meta.ID != id || validateConnectionInstanceMeta(meta) != nil {
		_ = s.quarantine(s.ConnectionInstancePath(id), "corrupt")
		_ = s.quarantine(s.ConnectionSnapshotPath(id), "corrupt")
		return ConnectionInstanceMeta{}, fmt.Errorf("invalid connection instance metadata %s", id)
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
	ReuseFromConnectionInstanceID     *string                 `json:"reuseFromConnectionInstanceId"`
	GenerationStatus                  string                  `json:"generationStatus"`
	GenerationError                   string                  `json:"generationError"`
	TmuxEnabled                       bool                    `json:"tmuxEnabled"`
	TmuxSessionName                   string                  `json:"tmuxSessionName"`
	TmuxPrefixKey                     string                  `json:"tmuxPrefixKey"`
	TmuxPrefixSource                  string                  `json:"tmuxPrefixSource"`
	Executions                        []legacyExecutionRecord `json:"executions,omitempty"`
	HostVerificationAssessment        string                  `json:"hostVerificationAssessment,omitempty"`
	ReconnectFromConnectionInstanceID *string                 `json:"reconnectFromConnectionInstanceId,omitempty"`
	RelaunchFromConnectionInstanceID  *string                 `json:"relaunchFromConnectionInstanceId,omitempty"`
	GenerationStaging                 string                  `json:"generationStaging,omitempty"`
}

func decodeConnectionInstanceMeta(data []byte) (ConnectionInstanceMeta, error) {
	var version struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return ConnectionInstanceMeta{}, err
	}
	switch version.FormatVersion {
	case LegacyConnectionFormatVersion:
		var current connectionMetaV1
		if err := decodeStrict(data, &current); err != nil {
			return ConnectionInstanceMeta{}, err
		}
		meta := ConnectionInstanceMeta{FormatVersion: ConnectionFormatVersion, ID: current.ID, BackendRuntimeID: current.BackendRuntimeID, ConnectionDefinitionID: current.ConnectionDefinitionID, Type: current.Type, Purpose: current.Purpose, SourceHostAlias: current.SourceHostAlias, Lifecycle: current.Lifecycle, SourceState: current.SourceState, AutomaticTitle: current.AutomaticTitle, TitleOverride: current.TitleOverride, InitialCwd: current.InitialCwd, Cwd: current.Cwd, Cols: current.Cols, Rows: current.Rows, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, ExitCode: current.ExitCode, ExitSignal: current.ExitSignal, ReuseFromConnectionInstanceID: current.ReuseFromConnectionInstanceID, GenerationStatus: current.GenerationStatus, GenerationError: current.GenerationError, TmuxEnabled: current.TmuxEnabled, TmuxSessionName: current.TmuxSessionName, TmuxPrefixKey: current.TmuxPrefixKey, TmuxPrefixSource: current.TmuxPrefixSource}
		meta.SyncEffectiveTitle()
		return meta, nil
	case ConnectionFormatVersion:
		var current connectionMetaV2
		if err := decodeStrict(data, &current); err != nil {
			return ConnectionInstanceMeta{}, err
		}
		meta := ConnectionInstanceMeta{FormatVersion: ConnectionFormatVersion, ID: current.ID, BackendRuntimeID: current.BackendRuntimeID, ConnectionDefinitionID: current.ConnectionDefinitionID, Type: current.Type, Purpose: current.Purpose, SourceHostAlias: current.SourceHostAlias, Lifecycle: current.Lifecycle, SourceState: current.SourceState, AutomaticTitle: current.AutomaticTitle, TitleOverride: current.TitleOverride, InitialCwd: current.InitialCwd, Cwd: current.Cwd, Cols: current.Cols, Rows: current.Rows, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt, ExitCode: current.ExitCode, ExitSignal: current.ExitSignal, ReuseFromConnectionInstanceID: current.ReuseFromConnectionInstanceID, GenerationStatus: current.GenerationStatus, GenerationError: current.GenerationError, TmuxEnabled: current.TmuxEnabled, TmuxSessionName: current.TmuxSessionName, TmuxPrefixKey: current.TmuxPrefixKey, TmuxPrefixSource: current.TmuxPrefixSource}
		meta.SyncEffectiveTitle()
		return meta, nil
	default:
		return ConnectionInstanceMeta{}, errors.New("unsupported connection instance format version")
	}
}

func (s *Store) ListConnectionInstances() ([]ConnectionInstanceMeta, error) {
	entries, err := os.ReadDir(s.ConnectionsDir)
	if err != nil {
		return nil, err
	}
	result := make([]ConnectionInstanceMeta, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.LoadConnectionInstance(entry.Name())
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

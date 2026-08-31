package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

const (
	maxStateBytes = 512 * 1024
	defaultMaxAge = 7 * 24 * time.Hour
)

var (
	ErrInvalidState    = errors.New("invalid agent state")
	ErrRuntimeMismatch = errors.New("agent state runtime mismatch")
)

func FilePath(home, runtimeID string) string {
	return filepath.Join(home, ".roaminal", "state", "agents", model.ProviderCodex, runtimeID+".json")
}

// Update appends one provider-adapted record and atomically publishes the
// resulting snapshot. The supplied index is intentionally ignored: the local
// file is the authority for monotonic allocation.
func Update(home string, info tmux.Info, componentVersion string, record model.StateRecord) (model.StateFile, error) {
	if err := validateInfo(info); err != nil {
		return model.StateFile{}, err
	}
	if err := validateState(record.State); err != nil {
		return model.StateFile{}, err
	}
	if strings.TrimSpace(componentVersion) == "" {
		componentVersion = "unknown"
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	} else {
		record.Timestamp = record.Timestamp.UTC()
	}
	runtimeID := tmux.RuntimeID(info)
	var result model.StateFile
	err := tmux.WithRuntimeLock(nil, home, info, func() error {
		current, err := readFile(FilePath(home, runtimeID))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if errors.Is(err, os.ErrNotExist) {
			current = newFile(info, runtimeID, componentVersion)
		} else if err := validateFile(current, info, runtimeID); err != nil {
			return err
		}
		if current.LatestIndex == ^uint64(0) {
			return errors.New("agent state index overflow")
		}
		record.Index = current.LatestIndex + 1
		current.LatestIndex = record.Index
		current.State = record.State
		current.ComponentVersion = componentVersion
		current.UpdatedAt = time.Now().UTC()
		current.Records = append(current.Records, record)
		if len(current.Records) > model.MaxStateRecords {
			current.Records = append([]model.StateRecord(nil), current.Records[len(current.Records)-model.MaxStateRecords:]...)
		}
		if err := validateFile(current, info, runtimeID); err != nil {
			return err
		}
		if err := writeFile(FilePath(home, runtimeID), current); err != nil {
			return err
		}
		result = current
		return nil
	})
	if err != nil {
		return model.StateFile{}, err
	}
	// Cleanup is deliberately best effort and outside the critical update
	// path. A stale runtime must never make a hook fail.
	_ = Cleanup(home, time.Now().UTC(), defaultMaxAge, runtimeID)
	return result, nil
}

func Read(home string, info tmux.Info) (model.StateFile, error) {
	if err := validateInfo(info); err != nil {
		return model.StateFile{}, err
	}
	runtimeID := tmux.RuntimeID(info)
	file, err := readFile(FilePath(home, runtimeID))
	if err != nil {
		return model.StateFile{}, err
	}
	if err := validateFile(file, info, runtimeID); err != nil {
		return model.StateFile{}, err
	}
	return file, nil
}

func Cleanup(home string, now time.Time, maxAge time.Duration, activeRuntimeID string) error {
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	dir := filepath.Dir(FilePath(home, "placeholder"))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		runtimeID := strings.TrimSuffix(entry.Name(), ".json")
		if runtimeID == activeRuntimeID || !validRuntimeID(runtimeID) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || now.Sub(info.ModTime()) < maxAge {
			continue
		}
		_ = os.Remove(path)
	}
	return nil
}

func newFile(info tmux.Info, runtimeID, componentVersion string) model.StateFile {
	return model.StateFile{
		SchemaVersion: model.SchemaVersion, Provider: model.ProviderCodex,
		ComponentVersion: componentVersion,
		Capabilities:     model.StateCapabilities{Running: true, Relax: true, Error: false},
		Tmux:             toModelTmux(info), RuntimeID: runtimeID, State: model.StateRelax,
		Records: []model.StateRecord{},
	}
}

func readFile(path string) (model.StateFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return model.StateFile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return model.StateFile{}, fmt.Errorf("%w: state file permissions are unsafe", ErrInvalidState)
	}
	file, err := os.Open(path)
	if err != nil {
		return model.StateFile{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil {
		return model.StateFile{}, err
	}
	if len(data) > maxStateBytes {
		return model.StateFile{}, fmt.Errorf("%w: state file is too large", ErrInvalidState)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result model.StateFile
	if err := decoder.Decode(&result); err != nil {
		return model.StateFile{}, fmt.Errorf("%w: %v", ErrInvalidState, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return model.StateFile{}, fmt.Errorf("%w: multiple JSON values", ErrInvalidState)
	}
	return result, nil
}

func writeFile(path string, value model.StateFile) error {
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path))))
	if err := ensurePrivateDir(root); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Join(root, "state")); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Join(root, "state", "agents")); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func validateInfo(info tmux.Info) error {
	if info.SessionName == "" || info.SessionID == "" || info.PaneID == "" || info.SocketFingerprint == "" || info.SessionCreated < 0 {
		return fmt.Errorf("%w: tmux identity is incomplete", ErrInvalidState)
	}
	return nil
}

func validateFile(file model.StateFile, info tmux.Info, runtimeID string) error {
	if file.SchemaVersion != model.SchemaVersion || file.Provider != model.ProviderCodex || file.RuntimeID != runtimeID || file.Tmux.SessionName != info.SessionName || file.Tmux.SessionID != info.SessionID || file.Tmux.SessionCreated != info.SessionCreated || file.Tmux.SocketFingerprint != info.SocketFingerprint {
		return ErrRuntimeMismatch
	}
	if strings.TrimSpace(file.ComponentVersion) == "" || !file.Capabilities.Running || !file.Capabilities.Relax || (file.State == model.StateError && !file.Capabilities.Error) {
		return fmt.Errorf("%w: state capabilities are invalid", ErrInvalidState)
	}
	if file.State != "" {
		if err := validateState(file.State); err != nil {
			return err
		}
	}
	if len(file.Records) > model.MaxStateRecords {
		return fmt.Errorf("%w: state history exceeds retention", ErrInvalidState)
	}
	var previous uint64
	for index, record := range file.Records {
		if record.Index == 0 || record.Index <= previous || record.Index > file.LatestIndex || record.Timestamp.IsZero() || validateState(record.State) != nil || !validRecordMetadata(record.EventName) || !validRecordMetadata(record.Source) || !validRecordMetadata(record.Reason) || !validRecordMetadata(record.ProviderSessionID) || !validRecordMetadata(record.TurnID) || !validRecordMetadata(record.ToolUseID) {
			return fmt.Errorf("%w: invalid state record %d", ErrInvalidState, index)
		}
		previous = record.Index
	}
	if len(file.Records) == 0 {
		if file.LatestIndex != 0 || file.State != model.StateRelax {
			return fmt.Errorf("%w: empty state history is inconsistent", ErrInvalidState)
		}
	} else {
		last := file.Records[len(file.Records)-1]
		if last.Index != file.LatestIndex || last.State != file.State || file.UpdatedAt.IsZero() {
			return fmt.Errorf("%w: latest state is inconsistent", ErrInvalidState)
		}
	}
	return nil
}

func validRecordMetadata(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validateState(value string) error {
	switch value {
	case model.StateRunning, model.StateRelax, model.StateError:
		return nil
	default:
		return fmt.Errorf("%w: unsupported state", ErrInvalidState)
	}
}

func toModelTmux(info tmux.Info) model.Tmux {
	return model.Tmux{SessionName: info.SessionName, SessionID: info.SessionID, SessionCreated: info.SessionCreated, PaneID: info.PaneID, SocketFingerprint: info.SocketFingerprint}
}

func validRuntimeID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		err = os.Mkdir(path, 0700)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("private state directory is unsafe")
	}
	if info.Mode().Perm() != 0700 {
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

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

package state

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

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

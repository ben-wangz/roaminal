package persistence

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func (r legacyReader) readDefinitionOptions(sshConfig string) (map[string]connectionoptions.Tmux, error) {
	options, err := r.read(filepath.Join(r.stateDir, "ssh-connection-options.yaml"), connectionoptions.MaxBytes, false)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("legacy SSH connection options cannot be read")
	}
	present := err == nil
	journal, err := r.read(filepath.Join(r.stateDir, "definition-mutation.json"), 2*(sshfs.ConfigMaxBytes+connectionoptions.MaxBytes)+4096, true)
	if err == nil {
		configBackup, optionsBackup, prepared, err := definition.LegacyMutationBackups(journal)
		if err != nil {
			return nil, err
		}
		if prepared {
			if err := checkLegacySSHConfig(sshConfig, configBackup); err != nil {
				return nil, err
			}
			options, present = optionsBackup.Data, optionsBackup.Exists
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("legacy SSH definition journal cannot be read")
	}
	if !present {
		return nil, nil
	}
	values, err := connectionoptions.DecodeLegacyOptions(options)
	if err != nil {
		return nil, errors.New("invalid legacy SSH connection options")
	}
	return values, nil
}

func checkLegacySSHConfig(path string, original connectionoptions.Snapshot) error {
	if path == "" {
		return errors.New("prepared legacy SSH mutation requires the matching SSH config copy (--ssh-config)")
	}
	data, err := readLegacyFile(path, sshfs.ConfigMaxBytes, false)
	if (!original.Exists && errors.Is(err, os.ErrNotExist)) || (original.Exists && err == nil && bytes.Equal(data, original.Data)) {
		return nil
	}
	// The v1 journal recorded only original bytes, not the intended revision.
	// Never guess whether a different file is a partial write or an outside edit.
	return errors.New("prepared legacy SSH mutation differs from its backup; reconcile the SSH config on the copy before import")
}

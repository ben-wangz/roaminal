package sshconfig

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

const configModeBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

// FileRevision is a stable fingerprint across restarts, unlike HTTP ETags,
// which are keyed to the serving repository. Missing and empty files differ.
type FileRevision struct {
	Exists bool
	Mode   os.FileMode
	Digest [sha256.Size]byte
}

func (v FileRevision) Valid() bool {
	if !v.Exists {
		return v == (FileRevision{})
	}
	return v.Mode&^configModeBits == 0 && v.Digest != [sha256.Size]byte{}
}

// PreparedMutation holds the exact rendered replacement. It is not rerendered
// after persisting the intent, so the expected fingerprint cannot drift.
type PreparedMutation struct {
	ETag   string
	Before FileRevision
	After  FileRevision
	data   []byte
}

func (r *Repository) PrepareMutation(ifMatch string, knownKeys map[string]bool, change Change) (PreparedMutation, error) {
	if strings.TrimSpace(ifMatch) == "" {
		return PreparedMutation{}, ErrPreconditionRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	doc, etag, source, err := r.Read(knownKeys)
	if err != nil {
		return PreparedMutation{}, err
	}
	if etag != ifMatch {
		return PreparedMutation{}, ErrPreconditionFailed
	}
	if !source.Writable && r.root != nil && !r.root.Available() {
		if err := r.root.EnsureDirectory(); err == nil {
			doc, etag, source, err = r.Read(knownKeys)
			if err != nil {
				return PreparedMutation{}, err
			}
			// Do not adopt a config that appeared while opening the directory.
			if source.Status != "missing" {
				return PreparedMutation{}, ErrPreconditionFailed
			}
		}
	}
	if !source.Writable {
		return PreparedMutation{}, fmt.Errorf("%w: %s", sshfs.ErrNotWritable, source.Reason)
	}
	before, info, err := r.mutationRevision()
	if err != nil {
		return PreparedMutation{}, err
	}
	if before.Exists != source.Readable || (before.Exists && sha256.Sum256(doc.Bytes) != before.Digest) ||
		r.etag(append(doc.Bytes, byte(0)), capabilityFingerprint(info, doc.Capability)) != etag {
		return PreparedMutation{}, ErrPreconditionFailed
	}
	data, err := change(doc, knownKeys)
	if err != nil {
		return PreparedMutation{}, err
	}
	if len(data) > sshfs.ConfigMaxBytes {
		return PreparedMutation{}, errors.New("ssh file exceeds size limit")
	}
	after := FileRevision{Exists: true, Mode: 0o600, Digest: sha256.Sum256(data)}
	return PreparedMutation{ETag: ifMatch, Before: before, After: after, data: data}, nil
}

// ApplyMutation publishes only if the file still matches the prepared revision.
// The bool distinguishes a rename from an error before the config changed.
func (r *Repository) ApplyMutation(plan PreparedMutation) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !plan.Before.Valid() || plan.After != (FileRevision{Exists: true, Mode: 0o600, Digest: sha256.Sum256(plan.data)}) {
		return false, errors.New("invalid prepared SSH config change")
	}
	check := func() error {
		live, _, err := r.mutationRevision()
		if err != nil {
			return err
		}
		if live != plan.Before {
			return ErrPreconditionFailed
		}
		return nil
	}
	if plan.Before == plan.After {
		// Only the options can change. Do not rewrite an identical SSH config.
		return false, check()
	}
	return r.root.AtomicReplaceChecked("config", plan.data, sshfs.ConfigMaxBytes, check)
}

func (r *Repository) MutationRevision() (FileRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	revision, _, err := r.mutationRevision()
	return revision, err
}

// SyncMutationRevision verifies the published revision and persists its rename
// before recovery commits the paired options.
func (r *Repository) SyncMutationRevision(expected FileRevision) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	live, _, err := r.mutationRevision()
	if err != nil {
		return err
	}
	if live != expected {
		return ErrPreconditionFailed
	}
	return r.root.SyncDirectory()
}

func (r *Repository) mutationRevision() (FileRevision, os.FileInfo, error) {
	info, err := r.root.Lstat("config")
	if errors.Is(err, os.ErrNotExist) {
		return FileRevision{}, nil, nil
	}
	if err != nil {
		return FileRevision{}, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > sshfs.ConfigMaxBytes {
		return FileRevision{}, nil, sshfs.ErrNotWritable
	}
	file, err := r.root.OpenNoFollow("config")
	if err != nil {
		return FileRevision{}, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !sameConfigFile(info, opened) {
		return FileRevision{}, nil, ErrPreconditionFailed
	}
	data, err := io.ReadAll(io.LimitReader(file, sshfs.ConfigMaxBytes+1))
	if err != nil {
		return FileRevision{}, nil, err
	}
	latest, err := r.root.Lstat("config")
	if err != nil || !sameConfigFile(info, latest) || int64(len(data)) != info.Size() {
		return FileRevision{}, nil, ErrPreconditionFailed
	}
	return FileRevision{Exists: true, Mode: info.Mode() & configModeBits, Digest: sha256.Sum256(data)}, info, nil
}

func sameConfigFile(first, last os.FileInfo) bool {
	return last.Mode().IsRegular() && os.SameFile(first, last) && first.Size() == last.Size() &&
		first.Mode() == last.Mode() && first.ModTime().Equal(last.ModTime())
}

package sshfs

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func randomName(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + "fallback"
	}
	return fmt.Sprintf("%s%x", prefix, raw[:])
}

// AtomicReplace writes a regular managed file in the fixed ssh directory. It
// never follows an existing target symlink and does not repair ownership or
// permissions supplied by a mounted Secret.
func (r *Root) AtomicReplace(name string, data []byte, max int) error {
	_, err := r.AtomicReplaceChecked(name, data, max, nil)
	return err
}

// AtomicReplaceChecked runs check immediately before publishing a staged
// regular file. The bool reports whether rename succeeded, even if directory
// sync fails. A checked write publishes a file with exact mode 0600.
func (r *Root) AtomicReplaceChecked(name string, data []byte, max int, check func() error) (bool, error) {
	if err := validateName(name); err != nil {
		return false, err
	}
	if len(data) > max {
		return false, errors.New("ssh file exceeds size limit")
	}
	ok, reason := r.CanWrite(name)
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrNotWritable, reason)
	}
	tmp := randomName(".roaminal-")
	f, err := r.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = r.root.Remove(tmp)
		}
	}()
	if check != nil {
		if err := f.Chmod(0o600); err != nil {
			_ = f.Close()
			return false, err
		}
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return false, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	// Re-check the destination immediately before rename. os.Root prevents
	// traversal outside the directory, while this rejects a replacement link.
	if info, err := r.root.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return false, ErrNotWritable
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if check != nil {
		if err := check(); err != nil {
			return false, err
		}
	}
	if err := r.root.Rename(tmp, name); err != nil {
		return false, err
	}
	cleanup = false
	return true, r.SyncDirectory()
}

// SyncDirectory persists renames in the directory held by this root.
func (r *Root) SyncDirectory() error {
	if !r.Available() {
		return ErrUnavailable
	}
	dir, err := r.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func syncDirectory(path string) error {
	dir, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (r *Root) EnsureDirectory() error {
	if r.Available() {
		return nil
	}
	if err := os.MkdirAll(r.name, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(r.name, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(r.name)
	if err != nil {
		return err
	}
	r.root = root
	return nil
}

// PromoteNoReplace atomically publishes a staged regular file without ever
// replacing an existing destination. The source and destination stay inside
// the same fixed root, so a hard link gives renameat2(RENAME_NOREPLACE)
// semantics even on filesystems where that syscall is unavailable.
func (r *Root) PromoteNoReplace(source, destination string) error {
	if err := validateName(source); err != nil {
		return err
	}
	if err := validateName(destination); err != nil {
		return err
	}
	if !r.Available() {
		return ErrUnavailable
	}
	info, err := r.root.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrUnsafePath
	}
	if _, err := r.root.Lstat(destination); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if ok, reason := r.CanWrite(destination); !ok {
		return fmt.Errorf("%w: %s", ErrNotWritable, reason)
	}
	if err := r.root.Link(source, destination); err != nil {
		return err
	}
	if err := syncDirectory(r.name); err != nil {
		_ = r.root.Remove(destination)
		return err
	}
	if err := r.root.Remove(source); err != nil {
		_ = r.root.Remove(destination)
		return err
	}
	return syncDirectory(r.name)
}

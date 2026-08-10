package connectionoptions

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

func (s *Store) Save(options map[string]Tmux) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(options)
}

func (s *Store) saveLocked(options map[string]Tmux) error {
	if len(options) == 0 {
		if info, err := os.Lstat(s.path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return ErrOptionsSymlink
			}
			if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	for alias, value := range options {
		if alias == "" || !value.Enabled || !ValidSessionName(value.SessionName) {
			return ErrInvalidSessionName
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(s.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrOptionsSymlink
	}
	if info, err := os.Lstat(s.path); err == nil {
		if ok, reason := s.canWrite(info); !ok {
			return fmt.Errorf("%w: %s", ErrOptionsNotWritable, reason)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if ok, reason := s.canWrite(nil); !ok {
		return fmt.Errorf("%w: %s", ErrOptionsNotWritable, reason)
	}
	aliases := make([]string, 0, len(options))
	for alias := range options {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	value := file{FormatVersion: FormatVersion, Connections: make(map[string]connectionSettings, len(options))}
	for _, alias := range aliases {
		option := options[alias]
		value.Connections[alias] = connectionSettings{Tmux: &tmuxSettings{Enabled: true, SessionName: option.SessionName}}
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return atomicWrite(s.path, data)
}

func (s *Store) canWrite(info os.FileInfo) (bool, string) {
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		return false, ErrOptionsSymlink.Error()
	}
	if info != nil && !info.Mode().IsRegular() {
		return false, "options path is not a regular file"
	}
	dir, err := os.Stat(filepath.Dir(s.path))
	if err != nil || !dir.IsDir() {
		return false, "state directory unavailable"
	}
	if info != nil && info.Mode().Perm()&0o022 != 0 {
		return false, "options file has unsafe permissions"
	}
	if info != nil {
		f, err := os.OpenFile(s.path, os.O_WRONLY, 0)
		if err != nil {
			return false, "options file is not writable"
		}
		_ = f.Close()
	}
	return true, ""
}

func atomicWrite(path string, data []byte) error {
	var random [8]byte
	_, _ = rand.Read(random[:])
	tmp := fmt.Sprintf("%s.%x.tmp", path, random)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrOptionsSymlink
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

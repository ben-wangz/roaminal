package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

func (s *Store) SaveAuth(file AuthFile) error {
	for _, session := range file.Sessions {
		if err := validateAuthSession(session); err != nil {
			return s.markError(err)
		}
	}
	file.FormatVersion = FormatVersion
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(filepath.Join(s.Root, "auth-sessions.json"), append(data, '\n')); err != nil {
		return s.markError(err)
	}
	s.degradedMu.Lock()
	s.globalError = false
	s.degradedMu.Unlock()
	return nil
}

func (s *Store) LoadAuth() (AuthFile, error) {
	path := filepath.Join(s.Root, "auth-sessions.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AuthFile{FormatVersion: FormatVersion, Sessions: []AuthSession{}}, nil
	}
	if err != nil {
		return AuthFile{}, err
	}
	var file AuthFile
	if err := decodeStrict(data, &file); err != nil || file.FormatVersion != FormatVersion {
		_ = s.quarantine(path, "corrupt")
		return AuthFile{FormatVersion: FormatVersion, Sessions: []AuthSession{}}, nil
	}
	for _, session := range file.Sessions {
		if err := validateAuthSession(session); err != nil {
			_ = s.quarantine(path, "corrupt")
			return AuthFile{FormatVersion: FormatVersion, Sessions: []AuthSession{}}, nil
		}
	}
	return file, nil
}

func (s *Store) quarantine(path, suffix string) error {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return os.Rename(path, path+"."+suffix+"."+stamp)
}

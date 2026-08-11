package persistence

import (
	"errors"
	"os"
	"path/filepath"
)

// AuditConnectionInstancePath and AuditConnectionSnapshotPath point at the
// single retained copy of a completed connection. Audit material is outside
// the active connection directory and is not included in ListConnectionInstances.
func (s *Store) AuditConnectionInstancePath(id string) string {
	return filepath.Join(s.AuditDir, "connection-instances", id, "metadata.json")
}

func (s *Store) AuditConnectionSnapshotPath(id string) string {
	return filepath.Join(s.AuditDir, "connection-instances", id, "terminal.snapshot")
}

// ArchiveConnectionInstance copies the latest active metadata and optional terminal
// snapshot into the audit area. It does not remove active data; callers must
// only call DeleteConnectionInstance after this method succeeds.
func (s *Store) ArchiveConnectionInstance(id string) error {
	if !uuidPattern.MatchString(id) {
		return s.markConnectionInstanceError(id, errors.New("invalid connection instance id"))
	}
	metadata, err := os.ReadFile(s.ConnectionInstancePath(id))
	if err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.AuditConnectionInstancePath(id)), 0o700); err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	if err := ensurePrivateDirectory(filepath.Dir(s.AuditConnectionInstancePath(id))); err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	if err := s.atomicWrite(s.AuditConnectionInstancePath(id), metadata); err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	snapshot, err := os.ReadFile(s.ConnectionSnapshotPath(id))
	if errors.Is(err, os.ErrNotExist) {
		s.clearConnectionInstanceError(id)
		return nil
	}
	if err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	if err := s.atomicWrite(s.AuditConnectionSnapshotPath(id), snapshot); err != nil {
		return s.markConnectionInstanceError(id, err)
	}
	s.clearConnectionInstanceError(id)
	return nil
}

package persistence

import (
	"errors"
	"os"
	"path/filepath"
)

// AuditSessionPath and AuditSnapshotPath point at the single retained copy of
// a completed connection. Audit material is intentionally outside the active
// connection directory and is not included in ListSessions.
func (s *Store) AuditSessionPath(id string) string {
	return filepath.Join(s.AuditDir, "connection-instances", id, "metadata.json")
}

func (s *Store) AuditSnapshotPath(id string) string {
	return filepath.Join(s.AuditDir, "connection-instances", id, "terminal.snapshot")
}

// ArchiveSession copies the latest active metadata and optional terminal
// snapshot into the audit area. It does not remove active data; callers must
// only call DeleteSession after this method succeeds.
func (s *Store) ArchiveSession(id string) error {
	if !uuidPattern.MatchString(id) {
		return s.markSessionError(id, errors.New("invalid session id"))
	}
	metadata, err := os.ReadFile(s.SessionPath(id))
	if err != nil {
		return s.markSessionError(id, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.AuditSessionPath(id)), 0o700); err != nil {
		return s.markSessionError(id, err)
	}
	if err := ensurePrivateDirectory(filepath.Dir(s.AuditSessionPath(id))); err != nil {
		return s.markSessionError(id, err)
	}
	if err := s.atomicWrite(s.AuditSessionPath(id), metadata); err != nil {
		return s.markSessionError(id, err)
	}
	snapshot, err := os.ReadFile(s.SnapshotPath(id))
	if errors.Is(err, os.ErrNotExist) {
		s.clearSessionError(id)
		return nil
	}
	if err != nil {
		return s.markSessionError(id, err)
	}
	if err := s.atomicWrite(s.AuditSnapshotPath(id), snapshot); err != nil {
		return s.markSessionError(id, err)
	}
	s.clearSessionError(id)
	return nil
}

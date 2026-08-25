package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

var uploadIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type uploadRecordFile struct {
	FormatVersion int                    `json:"formatVersion"`
	Record        domain.UploadJobRecord `json:"record"`
}

func (s *Store) uploadDir() string { return filepath.Join(s.Root, "uploads") }

func (s *Store) uploadPath(id string) string {
	if !uploadIDPattern.MatchString(id) {
		return ""
	}
	return filepath.Join(s.uploadDir(), id+".json")
}

func (s *Store) SaveUpload(ctx context.Context, record domain.UploadJobRecord) error {
	if err := checkPersistenceContext(ctx); err != nil {
		return err
	}
	if err := validateUploadRecord(record); err != nil {
		return s.markError(err)
	}
	path := s.uploadPath(record.ID)
	if path == "" {
		return s.markError(errors.New("invalid upload id"))
	}
	if err := os.MkdirAll(s.uploadDir(), 0o700); err != nil {
		return s.markError(fmt.Errorf("create upload repository: %w", err))
	}
	data, err := encodeJSON(uploadRecordFile{FormatVersion: StorageSchemaVersion, Record: record})
	if err != nil {
		return s.markError(err)
	}
	if err := s.atomicWrite(path, append(data, '\n')); err != nil {
		return s.markError(fmt.Errorf("save upload %s: %w", record.ID, err))
	}
	return nil
}

func (s *Store) LoadUpload(ctx context.Context, id string) (domain.UploadJobRecord, error) {
	if err := checkPersistenceContext(ctx); err != nil {
		return domain.UploadJobRecord{}, err
	}
	path := s.uploadPath(id)
	if path == "" {
		return domain.UploadJobRecord{}, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.UploadJobRecord{}, err
	}
	if err != nil {
		return domain.UploadJobRecord{}, fmt.Errorf("read upload %s: %w", id, err)
	}
	var file uploadRecordFile
	if err := decodeStrict(data, &file); err != nil {
		return domain.UploadJobRecord{}, fmt.Errorf("decode upload %s: %w", id, err)
	}
	if file.FormatVersion != StorageSchemaVersion {
		return domain.UploadJobRecord{}, fmt.Errorf("unsupported upload schema version %d", file.FormatVersion)
	}
	if err := validateUploadRecord(file.Record); err != nil {
		return domain.UploadJobRecord{}, fmt.Errorf("validate upload %s: %w", id, err)
	}
	return file.Record, nil
}

func (s *Store) DeleteUpload(ctx context.Context, id string) error {
	if err := checkPersistenceContext(ctx); err != nil {
		return err
	}
	path := s.uploadPath(id)
	if path == "" {
		return os.ErrNotExist
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete upload %s: %w", id, err)
	}
	return nil
}

func checkPersistenceContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func validateUploadRecord(record domain.UploadJobRecord) error {
	if !uploadIDPattern.MatchString(record.ID) || record.ConnectionInstanceID == "" || record.RootRevision == "" || record.TargetPath == "" {
		return errors.New("invalid upload identity")
	}
	switch record.Status {
	case "queued", "running", "completed", "failed", "partial-failure", "cancelled":
	default:
		return errors.New("invalid upload status")
	}
	switch record.ConflictPolicy {
	case "refuse", "overwrite", "update-if-newer":
	default:
		return errors.New("invalid upload conflict policy")
	}
	if record.BytesSent < 0 || record.BytesTotal < 0 || record.BytesSent > record.BytesTotal || record.CreatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("invalid upload counters or timestamps")
	}
	for _, file := range record.Files {
		if file.Part == "" || file.RelativePath == "" || path.IsAbs(file.RelativePath) || path.Clean(file.RelativePath) != file.RelativePath || file.RelativePath == "." || file.Size < 0 || file.StagedPath == "" {
			return errors.New("invalid upload file record")
		}
	}
	return nil
}

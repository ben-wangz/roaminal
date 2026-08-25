package filesystem

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type memoryUploadRepository struct {
	mu      sync.Mutex
	records map[string]domain.UploadJobRecord
}

func newMemoryUploadRepository() ports.UploadRepository {
	return &memoryUploadRepository{records: make(map[string]domain.UploadJobRecord)}
}

func (r *memoryUploadRepository) LoadUpload(ctx context.Context, id string) (domain.UploadJobRecord, error) {
	if err := contextError(ctx); err != nil {
		return domain.UploadJobRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[id]
	if !ok {
		return domain.UploadJobRecord{}, os.ErrNotExist
	}
	return cloneUploadRecord(record), nil
}

func (r *memoryUploadRepository) SaveUpload(ctx context.Context, record domain.UploadJobRecord) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	r.records[record.ID] = cloneUploadRecord(record)
	r.mu.Unlock()
	return nil
}

func (r *memoryUploadRepository) DeleteUpload(ctx context.Context, id string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.records, id)
	r.mu.Unlock()
	return nil
}

func contextError(ctx context.Context) error {
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

func (s *Service) persistUpload(job *uploadJob) error {
	if s.uploadRepo == nil {
		return nil
	}
	return s.uploadRepo.SaveUpload(context.Background(), uploadRecord(job))
}

func uploadRecord(job *uploadJob) domain.UploadJobRecord {
	status := job.snapshot()
	record := domain.UploadJobRecord{
		ID:                   status.UploadID,
		ConnectionInstanceID: job.instanceID,
		RootRevision:         job.root.Revision,
		TargetPath:           status.TargetPath,
		ConflictPolicy:       job.conflictPolicy,
		Status:               status.Status,
		Transport:            status.Transport,
		BytesSent:            status.BytesSent,
		BytesTotal:           status.BytesTotal,
		CurrentPath:          status.CurrentPath,
		StagingPath:          job.staging,
		CreatedAt:            job.createdAt,
		UpdatedAt:            job.updatedAt(),
		Failures:             make([]domain.UploadFailureRecord, 0, len(status.Failures)),
		Files:                make([]domain.UploadFileRecord, 0, len(job.files)),
	}
	for _, failure := range status.Failures {
		record.Failures = append(record.Failures, domain.UploadFailureRecord{Path: failure.Path, Code: failure.Code, Error: failure.Error})
	}
	for _, file := range job.files {
		record.Files = append(record.Files, domain.UploadFileRecord{Part: file.Manifest.Part, RelativePath: file.Manifest.RelativePath, Size: file.Manifest.Size, ModifiedAt: file.Manifest.ModifiedAt, StagedPath: file.Path})
	}
	return record
}

func (j *uploadJob) updatedAt() time.Time {
	if j.clock != nil {
		return j.clock.Now().UTC()
	}
	return systemclock.System{}.Now().UTC()
}

func uploadStatusFromRecord(record domain.UploadJobRecord) UploadStatus {
	status := UploadStatus{UploadID: record.ID, Status: record.Status, Transport: record.Transport, TargetPath: record.TargetPath, BytesSent: record.BytesSent, BytesTotal: record.BytesTotal, CurrentPath: record.CurrentPath, Failures: make([]UploadFailure, 0, len(record.Failures))}
	for _, failure := range record.Failures {
		status.Failures = append(status.Failures, UploadFailure{Path: failure.Path, Code: failure.Code, Error: failure.Error})
	}
	return status
}

func isUploadNotFound(err error) bool { return errors.Is(err, os.ErrNotExist) }

func cloneUploadRecord(record domain.UploadJobRecord) domain.UploadJobRecord {
	copyRecord := record
	copyRecord.Failures = append([]domain.UploadFailureRecord(nil), record.Failures...)
	copyRecord.Files = append([]domain.UploadFileRecord(nil), record.Files...)
	return copyRecord
}

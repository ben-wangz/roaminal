package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"regexp"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.UploadRepository = (*Store)(nil)

var (
	errUploadRepositoryUnavailable = errors.New("upload repository unavailable")
	uploadIDPattern                = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
)

type uploadRow struct {
	ID      string `gorm:"primaryKey;size:128"`
	Payload []byte `gorm:"not null"`
}

func (uploadRow) TableName() string { return "upload_jobs" }

func (s *Store) LoadUpload(ctx context.Context, id string) (domain.UploadJobRecord, error) {
	ctx = workspaceUploadContext(ctx)
	if err := ctx.Err(); err != nil {
		return domain.UploadJobRecord{}, err
	}
	if !uploadIDPattern.MatchString(id) {
		return domain.UploadJobRecord{}, os.ErrNotExist
	}
	var row uploadRow
	err := s.DB.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.UploadJobRecord{}, os.ErrNotExist
	}
	if err != nil {
		return domain.UploadJobRecord{}, workspaceUploadDBError(ctx, errUploadRepositoryUnavailable)
	}
	var record domain.UploadJobRecord
	if decodeWorkspaceUploadPayload(row.Payload, &record) != nil || record.ID != id {
		return domain.UploadJobRecord{}, errors.New("invalid upload repository record")
	}
	if err := validateUploadRecord(record); err != nil {
		return domain.UploadJobRecord{}, err
	}
	return record, nil
}

func (s *Store) SaveUpload(ctx context.Context, record domain.UploadJobRecord) error {
	ctx = workspaceUploadContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateUploadRecord(record); err != nil {
		return err
	}
	// Live staging is owned only by the running upload, not by the database.
	record.StagingPath = ""
	record.Files = append(record.Files[:0:0], record.Files...)
	for i := range record.Files {
		record.Files[i].StagedPath = ""
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return errors.New("encode upload repository record")
	}
	row := uploadRow{ID: record.ID, Payload: payload}
	err = s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload"}),
	}).Create(&row).Error
	if err != nil {
		return workspaceUploadDBError(ctx, errUploadRepositoryUnavailable)
	}
	return nil
}

func (s *Store) DeleteUpload(ctx context.Context, id string) error {
	ctx = workspaceUploadContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if !uploadIDPattern.MatchString(id) {
		return os.ErrNotExist
	}
	if err := s.DB.WithContext(ctx).Where("id = ?", id).Delete(&uploadRow{}).Error; err != nil {
		return workspaceUploadDBError(ctx, errUploadRepositoryUnavailable)
	}
	return nil
}

func (s *Store) interruptUploads(ctx context.Context, tx *gorm.DB) error {
	var rows []uploadRow
	return tx.FindInBatches(&rows, 100, func(batch *gorm.DB, _ int) error {
		store := &Store{DB: batch, driver: s.driver}
		for _, row := range rows {
			var record domain.UploadJobRecord
			if decodeWorkspaceUploadPayload(row.Payload, &record) != nil || row.ID != record.ID || validateUploadRecord(record) != nil {
				return errors.New("invalid upload repository record")
			}
			if domain.InterruptUpload(&record) {
				if err := store.SaveUpload(ctx, record); err != nil {
					return err
				}
			}
		}
		return nil
	}).Error
}

// Keep the same record validation as the file upload repository. These checks
// cover persisted metadata, not the staged file contents or remote filesystem.
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
		if file.Part == "" || file.RelativePath == "" || path.IsAbs(file.RelativePath) || path.Clean(file.RelativePath) != file.RelativePath || file.RelativePath == "." || file.Size < 0 {
			return errors.New("invalid upload file record")
		}
	}
	return nil
}

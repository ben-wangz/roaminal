package db

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.AuditRepository = (*Store)(nil)

type instanceAuditRow struct {
	ID       string `gorm:"primaryKey;size:36"`
	Metadata []byte `gorm:"not null"`
	Snapshot []byte
}

func (instanceAuditRow) TableName() string { return "connection_instance_audit" }

// ArchiveConnectionInstance copies the latest active metadata and optional
// snapshot without changing the active store.
func (s *Store) ArchiveConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) error {
	key := id.String()
	if err := checkInstanceID(ctx, key); err != nil {
		if errors.Is(err, errInstanceID) {
			return s.instanceFailure(key, err)
		}
		return err
	}
	return s.writeInstance(ctx, key, func(tx *gorm.DB) error { return copyInstanceToAudit(tx, key) })
}

// RetireConnectionInstance archives and removes active material in one
// transaction. It is safe to retry after success without replacing the audit.
func (s *Store) RetireConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) error {
	key := id.String()
	if err := checkInstanceID(ctx, key); err != nil {
		return err
	}
	return s.writeInstance(ctx, key, func(tx *gorm.DB) error {
		err := copyInstanceToAudit(tx, key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var audit instanceAuditRow
			return tx.Select("id").Where("id = ?", key).Take(&audit).Error
		}
		if err != nil {
			return err
		}
		return deleteActiveInstance(tx, key)
	})
}

func copyInstanceToAudit(tx *gorm.DB, id string) error {
	var active instanceRow
	if err := tx.Where("id = ?", id).Take(&active).Error; err != nil {
		return err
	}
	snapshot, err := selectBoundedSnapshot(tx, id)
	row := instanceAuditRow{ID: id, Metadata: active.Metadata, Snapshot: snapshot}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Missing active snapshot data does not erase an earlier audit snapshot.
		return upsertInstanceAudit(tx, &row, "metadata")
	}
	if err != nil {
		return err
	}
	return upsertInstanceAudit(tx, &row, "metadata", "snapshot")
}

func upsertInstanceAudit(tx *gorm.DB, row *instanceAuditRow, columns ...string) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(row).Error
}

// ImportArchivedConnectionInstance writes audit-only data without creating an
// active instance. A nil header denotes an absent snapshot, not an empty one.
func (s *Store) ImportArchivedConnectionInstance(ctx context.Context, meta domain.ConnectionInstanceMeta, header *domain.SnapshotHeader, payload []byte) error {
	if err := instanceContextError(ctx); err != nil {
		return err
	}
	metadata, err := encodeInstanceMetadata(meta)
	if err != nil {
		return s.instanceFailure(meta.ID, err)
	}
	row := instanceAuditRow{ID: meta.ID, Metadata: metadata}
	if header == nil && len(payload) != 0 {
		return s.instanceFailure(meta.ID, errors.New("snapshot header is required for audit payload"))
	}
	if header != nil {
		row.Snapshot, err = encodeInstanceSnapshot(*header, payload)
		if err != nil {
			return s.instanceFailure(meta.ID, err)
		}
	}
	return s.writeInstance(ctx, meta.ID, func(tx *gorm.DB) error {
		return upsertInstanceAudit(tx, &row, "metadata", "snapshot")
	})
}

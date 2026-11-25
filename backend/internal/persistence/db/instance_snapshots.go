package db

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.TerminalSnapshotRepository = (*Store)(nil)

// Match the legacy snapshot decoder's total envelope limit.
const encodedSnapshotLimit = persistence.SnapshotMaxSize + 4096

var errSnapshotSize = errors.New("snapshot exceeds 256 MiB")

type instanceSnapshotRow struct {
	ID   string `gorm:"primaryKey;size:36"`
	Data []byte `gorm:"not null"`
}

func (instanceSnapshotRow) TableName() string { return "terminal_snapshots" }

func encodeInstanceSnapshot(header domain.SnapshotHeader, payload []byte) ([]byte, error) {
	data, err := persistence.EncodeSnapshot(header, payload)
	if err != nil {
		return nil, err
	}
	if len(data) > encodedSnapshotLimit {
		return nil, errSnapshotSize
	}
	return data, nil
}

func (s *Store) SaveSnapshot(ctx context.Context, id domain.ConnectionInstanceID, header domain.SnapshotHeader, payload []byte) error {
	key := id.String()
	if err := checkInstanceID(ctx, key); err != nil {
		if errors.Is(err, errInstanceID) {
			return s.instanceFailure(key, err)
		}
		return err
	}
	data, err := encodeInstanceSnapshot(header, payload)
	if err != nil {
		return s.instanceFailure(key, err)
	}
	return s.writeInstance(ctx, key, func(tx *gorm.DB) error {
		row := instanceSnapshotRow{ID: key, Data: data}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"data"}),
		}).Create(&row).Error
	})
}

func (s *Store) LoadSnapshot(ctx context.Context, id domain.ConnectionInstanceID) (domain.SnapshotHeader, []byte, error) {
	key := id.String()
	if err := checkInstanceID(ctx, key); err != nil {
		return domain.SnapshotHeader{}, nil, err
	}
	data, err := selectBoundedSnapshot(s.DB.WithContext(ctx), key)
	if err := s.instanceReadError(ctx, key, err); err != nil {
		return domain.SnapshotHeader{}, nil, err
	}
	header, payload, err := persistence.DecodeSnapshot(data)
	if err != nil {
		return domain.SnapshotHeader{}, nil, s.instanceFailure(key, err)
	}
	return header, payload, nil
}

func selectBoundedSnapshot(tx *gorm.DB, id string) ([]byte, error) {
	var row instanceSnapshotRow
	// Bound the transfer before asking the driver to materialize the blob.
	// Oversized data is distinct from a missing row.
	err := tx.Select("CASE WHEN octet_length(data) <= ? THEN data END AS data", encodedSnapshotLimit).
		Where("id = ?", id).Take(&row).Error
	if err != nil {
		return nil, err
	}
	if row.Data == nil {
		return nil, errSnapshotSize
	}
	return row.Data, nil
}

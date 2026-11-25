package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.ConnectionInstanceRepository = (*Store)(nil)

var (
	instanceIDPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	errInstanceID       = errors.New("invalid connection instance id")
	errInstanceMetadata = errors.New("invalid connection instance metadata")
)

type instanceRow struct {
	ID       string `gorm:"primaryKey;size:36"`
	Metadata []byte `gorm:"not null"`
}

func (instanceRow) TableName() string { return "connection_instances" }

func instanceModels() []any {
	return []any{&instanceRow{}, &instanceSnapshotRow{}, &instanceAuditRow{}}
}

type instanceMetadata struct {
	FormatVersion int `json:"formatVersion"`
	domain.ConnectionInstanceMeta
}

func encodeInstanceMetadata(meta domain.ConnectionInstanceMeta) ([]byte, error) {
	if meta.FormatVersion != 0 && meta.FormatVersion != persistence.ConnectionFormatVersion {
		return nil, errors.New("unsupported connection instance format version")
	}
	if meta.AutomaticTitle == "" && meta.TitleOverride == nil {
		meta.AutomaticTitle = meta.Title
	}
	meta.SyncEffectiveTitle()
	if err := domain.ValidateConnectionInstanceMeta(meta); err != nil {
		return nil, err
	}
	data, err := json.Marshal(instanceMetadata{FormatVersion: persistence.ConnectionFormatVersion, ConnectionInstanceMeta: meta})
	if err != nil {
		return nil, errInstanceMetadata
	}
	return data, nil
}

func decodeInstanceMetadata(row instanceRow) (domain.ConnectionInstanceMeta, error) {
	var record instanceMetadata
	decoder := json.NewDecoder(bytes.NewReader(row.Metadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil || record.FormatVersion != persistence.ConnectionFormatVersion || record.ID != row.ID {
		return domain.ConnectionInstanceMeta{}, errInstanceMetadata
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ConnectionInstanceMeta{}, errInstanceMetadata
	}
	meta := record.ConnectionInstanceMeta
	meta.FormatVersion = record.FormatVersion
	meta.SyncEffectiveTitle()
	if err := domain.ValidateConnectionInstanceMeta(meta); err != nil {
		return domain.ConnectionInstanceMeta{}, errInstanceMetadata
	}
	return meta, nil
}

func (s *Store) ListConnectionInstances(ctx context.Context) ([]domain.ConnectionInstanceMeta, error) {
	if err := instanceContextError(ctx); err != nil {
		return nil, err
	}
	var rows []instanceRow
	if err := instanceDatabaseError(ctx, s.DB.WithContext(ctx).Order("id").Find(&rows).Error); err != nil {
		if errors.Is(err, errInstanceDatabase) {
			s.instanceHealth.setListFailed(true)
		}
		return nil, err
	}
	result := make([]domain.ConnectionInstanceMeta, 0, len(rows))
	for _, row := range rows {
		meta, err := decodeInstanceMetadata(row)
		if err != nil {
			// Preserve unreadable records for recovery, but do not expose them
			// as active instances to the runtime.
			s.instanceHealth.set(row.ID, true)
			continue
		}
		result = append(result, meta)
	}
	s.instanceHealth.setListFailed(false)
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) LoadConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) (domain.ConnectionInstanceMeta, error) {
	key := id.String()
	if err := checkInstanceID(ctx, key); err != nil {
		return domain.ConnectionInstanceMeta{}, err
	}
	var row instanceRow
	if err := s.instanceReadError(ctx, key, s.DB.WithContext(ctx).Where("id = ?", key).Take(&row).Error); err != nil {
		return domain.ConnectionInstanceMeta{}, err
	}
	meta, err := decodeInstanceMetadata(row)
	if err != nil {
		return domain.ConnectionInstanceMeta{}, s.instanceFailure(key, err)
	}
	return meta, nil
}

func (s *Store) SaveConnectionInstance(ctx context.Context, meta domain.ConnectionInstanceMeta) error {
	if err := instanceContextError(ctx); err != nil {
		return err
	}
	data, err := encodeInstanceMetadata(meta)
	if err != nil {
		return s.instanceFailure(meta.ID, err)
	}
	return s.writeInstance(ctx, meta.ID, func(tx *gorm.DB) error {
		row := instanceRow{ID: meta.ID, Metadata: data}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"metadata"}),
		}).Create(&row).Error
	})
}

func (s *Store) DeleteConnectionInstance(ctx context.Context, id domain.ConnectionInstanceID) error {
	key := id.String()
	if err := checkInstanceID(ctx, key); err != nil {
		return err
	}
	return s.writeInstance(ctx, key, func(tx *gorm.DB) error { return deleteActiveInstance(tx, key) })
}

func deleteActiveInstance(tx *gorm.DB, id string) error {
	if err := tx.Where("id = ?", id).Delete(&instanceSnapshotRow{}).Error; err != nil {
		return err
	}
	return tx.Where("id = ?", id).Delete(&instanceRow{}).Error
}

func (s *Store) writeInstance(ctx context.Context, id string, write func(*gorm.DB) error) error {
	if err := instanceContextError(ctx); err != nil {
		return err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Snapshot writes and retirement share the same lock bucket, even
		// before a particular instance has an active row to lock.
		if err := lockRepository(tx, "instance-"+id[:1]); err != nil {
			return err
		}
		return write(tx)
	}, repositoryTxOptions(s.DB)...)
	err = instanceDatabaseError(ctx, err)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		s.instanceHealth.set(id, err != nil)
	}
	return err
}

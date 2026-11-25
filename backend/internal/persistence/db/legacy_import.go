package db

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"gorm.io/gorm"
)

var (
	errLegacyTarget = errors.New("file import requires an unused database and a consistent source")
	errLegacyVerify = errors.New("database records do not match the inspected legacy source; do not start serving")
)

// ImportLegacy applies and verifies a stopped file-store snapshot. The
// fingerprint is committed with the records, never ahead of them.
func (s *Store) ImportLegacy(ctx context.Context, data persistence.LegacyData) error {
	return s.legacyTransaction(ctx, data, true)
}

func (s *Store) VerifyLegacy(ctx context.Context, data persistence.LegacyData) error {
	return s.legacyTransaction(ctx, data, false)
}

func (s *Store) legacyTransaction(ctx context.Context, data persistence.LegacyData, apply bool) error {
	if !validHex(data.Fingerprint, 32) {
		return errLegacyTarget
	}
	var operationErr error
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operationErr = applyOrVerifyLegacy(ctx, tx, s.driver, data, apply)
		return operationErr
	}, repositoryTxOptions(s.DB)...)
	if err != nil {
		if operationErr != nil {
			return operationErr
		}
		return errors.New("legacy database transaction could not be committed")
	}
	return nil
}

func applyOrVerifyLegacy(ctx context.Context, tx *gorm.DB, driver string, data persistence.LegacyData, apply bool) error {
	row, err := lockInstallation(tx)
	if err != nil {
		return errInstallation
	}
	if row.Served || row.TransferFingerprint != "" {
		return errLegacyTarget
	}
	store := &Store{DB: tx, driver: driver}
	if row.LegacyFingerprint == data.Fingerprint {
		return store.verifyLegacyRows(ctx, data)
	}
	if row.LegacyFingerprint != "" || !apply || requireEmptyApplication(tx) != nil {
		return errLegacyTarget
	}
	if err := store.writeLegacy(ctx, data); err != nil {
		return err
	}
	if err := store.verifyLegacyRows(ctx, data); err != nil {
		return err
	}
	result := tx.Model(&row).Update("legacy_fingerprint", data.Fingerprint)
	if result.Error != nil || result.RowsAffected != 1 {
		return errInstallation
	}
	return nil
}

func (s *Store) writeLegacy(ctx context.Context, data persistence.LegacyData) error {
	if data.Enrollment != nil {
		if err := s.SaveEnrollment(ctx, *data.Enrollment); err != nil {
			return err
		}
	}
	if err := s.SaveAuth(ctx, data.Auth); err != nil {
		return err
	}
	for id, layout := range data.Layouts {
		if err := s.ImportWorkspaceLayout(ctx, domain.AuthenticationSessionID(id), layout); err != nil {
			return err
		}
	}
	if err := importMessages(s.DB, data.MessageState, data.Messages); err != nil {
		return err
	}
	if err := importPushSubscriptions(s.DB, data.Subscriptions); err != nil {
		return err
	}
	if err := importNotificationPreferences(s.DB, data.Preferences); err != nil {
		return err
	}
	for key, endpoint := range data.Endpoints {
		if err := s.ImportAgentEndpointWithKey(ctx, key, endpoint); err != nil {
			return err
		}
	}
	if err := s.ConnectionOptions().Replace(ctx, data.Options); err != nil {
		return err
	}
	for _, upload := range data.Uploads {
		if err := s.SaveUpload(ctx, upload); err != nil {
			return err
		}
	}
	for _, instance := range data.Instances {
		if err := s.writeLegacyInstance(ctx, instance); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) writeLegacyInstance(ctx context.Context, instance persistence.LegacyInstance) error {
	header, payload, err := instance.Snapshot()
	if err != nil {
		return err
	}
	if instance.Archived {
		return s.ImportArchivedConnectionInstance(ctx, instance.Metadata, header, payload)
	}
	if err := s.SaveConnectionInstance(ctx, instance.Metadata); err != nil {
		return err
	}
	if header != nil {
		return s.SaveSnapshot(ctx, domain.ConnectionInstanceID(instance.Metadata.ID), *header, payload)
	}
	return nil
}

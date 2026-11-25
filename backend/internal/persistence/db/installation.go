package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const primaryKey = "primary"

var (
	errInstallation   = errors.New("database installation record unavailable or invalid")
	ErrImportRequired = errors.New("legacy application state requires a matching completed database import")
	ErrServingBinding = errors.New("database is already bound to another Roaminal state directory")
)

type repositoryLock struct {
	Key string `gorm:"primaryKey;size:32"`
}

func (repositoryLock) TableName() string { return "repository_locks" }

func seedRepositoryLocks(tx *gorm.DB) error {
	locks := []repositoryLock{{Key: "auth"}, {Key: pushSubscriptionLock}, {Key: notificationPreferenceLock}}
	for _, bucket := range "0123456789abcdef" {
		locks = append(locks, repositoryLock{Key: "instance-" + string(bucket)})
	}
	return tx.Create(&locks).Error
}

func lockRepository(tx *gorm.DB, key string) error {
	var lock repositoryLock
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&lock, &repositoryLock{Key: key}).Error
}

type installationRow struct {
	Key                 string `gorm:"primaryKey;size:32"`
	ID                  string `gorm:"not null;size:32"`
	LegacyFingerprint   string `gorm:"not null;size:64"`
	TransferFingerprint string `gorm:"not null;size:64"`
	TransferSourceID    string `gorm:"not null;size:32"`
	ServingOwner        string `gorm:"not null;size:32"`
	Served              bool   `gorm:"not null"`
}

func (installationRow) TableName() string { return "database_installation" }

func seedInstallation(tx *gorm.DB) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	return tx.Create(&installationRow{Key: primaryKey, ID: hex.EncodeToString(id[:])}).Error
}

func validHex(value string, bytes int) bool {
	if len(value) != 2*bytes || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func readInstallation(query *gorm.DB) (installationRow, error) {
	var row installationRow
	if err := query.Take(&row, &installationRow{Key: primaryKey}).Error; err != nil || !validHex(row.ID, 16) ||
		(row.LegacyFingerprint != "" && !validHex(row.LegacyFingerprint, 32)) ||
		(row.TransferFingerprint != "" && !validHex(row.TransferFingerprint, 32)) ||
		(row.TransferSourceID != "" && !validHex(row.TransferSourceID, 16)) ||
		((row.TransferSourceID == "") != (row.TransferFingerprint == "")) ||
		(row.Served && !validHex(row.ServingOwner, 16)) || (!row.Served && row.ServingOwner != "") {
		return installationRow{}, errInstallation
	}
	return row, nil
}

func lockInstallation(tx *gorm.DB) (installationRow, error) {
	return readInstallation(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
}

// Installation describes the target without exposing credentials or record contents.
type Installation struct {
	ID                string
	LegacyFingerprint string
	TransferSourceID  string
	ServingOwner      string
	Served            bool
}

func (s *Store) Installation(ctx context.Context) (Installation, error) {
	row, err := readInstallation(s.DB.WithContext(ctx))
	return Installation{ID: row.ID, LegacyFingerprint: row.LegacyFingerprint, TransferSourceID: row.TransferSourceID, ServingOwner: row.ServingOwner, Served: row.Served}, err
}

// PrepareServing serializes first startup against offline import. Once a target
// has served, its source files are only evidence, not a live dependency.
func (s *Store) PrepareServing(ctx context.Context, owner string, legacyPresent bool, fingerprint string) error {
	if !validHex(owner, 16) {
		return errInstallation
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockInstallation(tx)
		if err != nil {
			return err
		}
		if row.Served && row.ServingOwner != owner {
			return ErrServingBinding
		}
		if !row.Served && legacyPresent && (row.LegacyFingerprint == "" || row.LegacyFingerprint != fingerprint) {
			return ErrImportRequired
		}
		if !row.Served && row.TransferFingerprint != "" {
			if err := verifyInstalledTransfer(tx, row); err != nil {
				return err
			}
		}
		if err := s.initializeMessageNotifications(tx); err != nil {
			return err
		}
		if err := s.interruptUploads(ctx, tx); err != nil {
			return err
		}
		if row.Served {
			return nil
		}
		return tx.Model(&row).Updates(map[string]any{"served": true, "serving_owner": owner}).Error
	}, repositoryTxOptions(s.DB)...)
	if err != nil && !errors.Is(err, ErrImportRequired) && !errors.Is(err, ErrServingBinding) && !errors.Is(err, ErrTransferVerification) {
		return errInstallation
	}
	return err
}

func requireEmptyApplication(tx *gorm.DB) error {
	for _, model := range applicationModels() {
		switch model.(type) {
		case *installationRow, *repositoryLock, *messageStateRow:
			continue
		}
		if err := requireEmptyRepository(tx, model); err != nil {
			return err
		}
	}
	for _, model := range []any{&enrollmentRow{}, &authRow{}} {
		if err := requireEmptyRepository(tx, model); err != nil {
			return err
		}
	}
	var states int64
	if err := tx.Model(&messageStateRow{}).Count(&states).Error; err != nil || states != 1 {
		return errors.New("import destination message state is unavailable")
	}
	messageState, err := lockMessageState(tx)
	if err != nil || messageState.LatestSequence != 0 || messageState.Revision != 0 || messageState.ReadThroughSequence != 0 {
		return errors.New("import destination message state is not empty or unavailable")
	}
	return nil
}

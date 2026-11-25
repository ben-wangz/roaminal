package db

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	errTransferDatabase = errors.New("database transfer transaction failed")
	errTransferTarget   = errors.New("database transfer requires a new, empty destination")
	errTransferPending  = errors.New("resolve pending connection-definition mutations before exporting")
	// ErrTransferVerification means the destination no longer matches the import.
	ErrTransferVerification = errors.New("database transfer destination does not match the verified backup")
)

type transferManifest struct {
	SchemaVersion     int    `json:"schemaVersion"`
	SourceID          string `json:"sourceId"`
	LegacyFingerprint string `json:"legacyFingerprint,omitempty"`
}

// Export writes a versioned, checksummed database backup. The caller must keep
// the application offline, including the files that DB state refers to. The
// backup contains credentials and must be protected like the database itself.
// The checksum verifies integrity, not authenticity. A complete footer is only
// written after the database snapshot is closed.
func (s *Store) Export(ctx context.Context, w io.Writer) error {
	if w == nil {
		return errTransferWrite
	}
	out := newTransferOutput(ctx, w)
	var options []*sql.TxOptions
	if s.DB.Dialector.Name() == Postgres {
		options = []*sql.TxOptions{{Isolation: sql.LevelRepeatableRead, ReadOnly: true}}
	}
	err := s.transferTransaction(ctx, options, func(tx *gorm.DB) error {
		source, err := readInstallation(tx)
		if err != nil {
			return errTransferDatabase
		}
		manifest := transferManifest{SchemaVersion: schemaVersion, SourceID: source.ID, LegacyFingerprint: source.LegacyFingerprint}
		return writeTransferSnapshot(tx, out, manifest)
	})
	if err != nil {
		return err
	}
	return out.finish()
}

func writeTransferSnapshot(tx *gorm.DB, out *transferOutput, manifest transferManifest) error {
	var pending int64
	if err := tx.Model(&definitionMutationRow{}).Count(&pending).Error; err != nil {
		return errTransferDatabase
	}
	if pending != 0 {
		return errTransferPending
	}
	if err := out.writeManifest(manifest); err != nil {
		return err
	}
	var state transferState
	for _, table := range transferTables {
		if err := table.export(tx, out, &state); err != nil {
			return err
		}
	}
	return nil
}

// Import restores a complete transfer in a single transaction. It never
// overwrites application rows or changes the target's installation ID. An exact
// retry is a no-op only if the unserved destination still matches the backup.
func (s *Store) Import(ctx context.Context, r io.Reader) error {
	if r == nil {
		return errTransferRead
	}
	in := newTransferInput(ctx, r)
	return s.transferTransaction(ctx, repositoryTxOptions(s.DB), func(tx *gorm.DB) error {
		target, err := lockInstallation(tx)
		if err != nil {
			return errTransferDatabase
		}
		if target.Served {
			return errTransferTarget
		}
		manifest, err := in.readManifest()
		if err != nil {
			return err
		}
		if manifest.SourceID == target.ID {
			return errTransferTarget
		}
		retry := target.TransferFingerprint != ""
		if retry {
			if target.TransferSourceID != manifest.SourceID || target.LegacyFingerprint != manifest.LegacyFingerprint {
				return errTransferTarget
			}
		} else if target.LegacyFingerprint != "" || requireEmptyApplication(tx) != nil {
			return errTransferTarget
		}
		var state transferState
		for _, table := range transferTables {
			if err := table.restore(tx, in, &state, retry); err != nil {
				return err
			}
		}
		digest, err := in.finish()
		if err != nil {
			return err
		}
		if retry && digest != target.TransferFingerprint {
			return errTransferTarget
		}
		if err := verifyTransferSnapshot(tx, manifest, digest); err != nil {
			return err
		}
		if retry {
			return nil
		}
		result := tx.Model(&target).Updates(map[string]any{
			"legacy_fingerprint": manifest.LegacyFingerprint, "transfer_fingerprint": digest, "transfer_source_id": manifest.SourceID,
		})
		if result.Error != nil || result.RowsAffected != 1 {
			return errTransferDatabase
		}
		return nil
	})
}

// VerifyTransfer checks the current contents of an unserved logical restore.
// The source file is not needed: import already verified and recorded its digest.
func (s *Store) VerifyTransfer(ctx context.Context) error {
	return s.transferTransaction(ctx, repositoryTxOptions(s.DB), func(tx *gorm.DB) error {
		target, err := lockInstallation(tx)
		if err != nil {
			return errTransferDatabase
		}
		if target.Served || target.TransferFingerprint == "" {
			return errTransferTarget
		}
		return verifyInstalledTransfer(tx, target)
	})
}

func verifyInstalledTransfer(tx *gorm.DB, target installationRow) error {
	manifest := transferManifest{SchemaVersion: schemaVersion, SourceID: target.TransferSourceID, LegacyFingerprint: target.LegacyFingerprint}
	return verifyTransferSnapshot(tx, manifest, target.TransferFingerprint)
}

func verifyTransferSnapshot(tx *gorm.DB, manifest transferManifest, digest string) error {
	// Re-encode the portable rows with the original manifest, not the target's
	// identity. Hash the stream without storing a second copy of the backup.
	out := newTransferOutput(tx.Statement.Context, io.Discard)
	if err := writeTransferSnapshot(tx, out, manifest); err != nil {
		if errors.Is(err, errTransferFormat) || errors.Is(err, errTransferPending) {
			return ErrTransferVerification
		}
		return err
	}
	if err := out.finish(); err != nil {
		return err
	}
	if hex.EncodeToString(out.digest.Sum(nil)) != digest {
		return ErrTransferVerification
	}
	return nil
}

func (s *Store) transferTransaction(ctx context.Context, options []*sql.TxOptions, run func(*gorm.DB) error) error {
	// This operation handles credentials and provider data. Never surface
	// driver, scanner, or writer diagnostics that may include those values.
	db := s.DB.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	var operationErr error
	err := db.Transaction(func(tx *gorm.DB) error {
		operationErr = run(tx)
		return operationErr
	}, options...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if operationErr != nil {
		return operationErr
	}
	if err != nil {
		return errTransferDatabase
	}
	return nil
}

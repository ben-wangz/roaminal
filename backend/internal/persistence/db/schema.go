package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const schemaVersion = 2

type schemaMigration struct {
	Version   int       `gorm:"primaryKey;autoIncrement:false"`
	AppliedAt time.Time `gorm:"not null"`
}

func (schemaMigration) TableName() string { return "schema_migrations" }

type enrollmentRow struct {
	Key                  string `gorm:"primaryKey;size:32"`
	FormatVersion        int    `gorm:"not null"`
	EnrollmentID         string `gorm:"not null;size:64"`
	Secret               string `gorm:"not null;size:256"`
	LastAcceptedTimeStep int64  `gorm:"not null"`
}

func (enrollmentRow) TableName() string { return "totp_enrollment" }

type authRow struct {
	ID                  string    `gorm:"primaryKey;size:64"`
	PasswordFingerprint string    `gorm:"not null"`
	EnrollmentID        string    `gorm:"not null;size:64"`
	RefreshTokenHash    string    `gorm:"not null"`
	CreatedAt           time.Time `gorm:"not null"`
	LastSeenAt          time.Time `gorm:"not null"`
	RefreshExpiresAt    time.Time `gorm:"not null"`
	RotatedAt           time.Time `gorm:"not null"`
	UserAgent           string    `gorm:"not null"`
	Submicroseconds     int64     `gorm:"not null;default:0"`
}

func (authRow) TableName() string { return "auth_sessions" }

func applicationModels() []any {
	models := []any{&repositoryLock{}, &installationRow{}}
	models = append(models, workspaceUploadModels()...)
	models = append(models, instanceModels()...)
	models = append(models, messageNotificationModels()...)
	return append(models, agentOptionsModels()...)
}

func (s *Store) migrate(ctx context.Context) error {
	migrate := func() error {
		return s.DB.WithContext(ctx).Transaction(migrateSchema, repositoryTxOptions(s.DB)...)
	}
	err := migrate()
	// Two PostgreSQL transactions can both see a missing migration table.
	// The catalog rejects one creation; restart that entire transaction so
	// it can lock the now-committed version row. No DDL is retried in place.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "42P07" || pgErr.Code == "23505") {
		err = migrate()
	}
	if err != nil {
		return errors.New("database schema unavailable or unsupported")
	}
	return nil
}

func migrateSchema(tx *gorm.DB) error {
	m := tx.Migrator()
	if !m.HasTable(&schemaMigration{}) {
		if m.HasTable(&enrollmentRow{}) || m.HasTable(&authRow{}) {
			return errors.New("unversioned authentication schema")
		}
		if err := m.CreateTable(&schemaMigration{}, &enrollmentRow{}, &authRow{}); err != nil {
			return err
		}
		if err := recordSchemaVersion(tx, 1); err != nil {
			return err
		}
	}
	var first schemaMigration
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&first, "version = ?", 1).Error; err != nil {
		return err
	}
	version, err := installedSchemaVersion(tx)
	if err != nil {
		return err
	}
	if !m.HasTable(&enrollmentRow{}) || !m.HasTable(&authRow{}) {
		return errors.New("incomplete authentication schema")
	}
	if version == 1 {
		if err := migrateApplicationTables(tx); err != nil {
			return err
		}
	}
	return checkApplicationSchema(tx)
}

func installedSchemaVersion(query *gorm.DB) (int, error) {
	var versions []schemaMigration
	if err := query.Order("version").Limit(schemaVersion + 1).Find(&versions).Error; err != nil {
		return 0, err
	}
	for i, v := range versions {
		if v.Version != i+1 || v.Version > schemaVersion {
			return 0, errors.New("unsupported or incomplete database schema")
		}
	}
	return len(versions), nil
}

func checkSchema(query *gorm.DB) error {
	version, err := installedSchemaVersion(query)
	if err != nil || version != schemaVersion || checkApplicationSchema(query) != nil {
		return errors.New("database must already exist with a supported schema")
	}
	return nil
}

func checkApplicationSchema(query *gorm.DB) error {
	m := query.Migrator()
	if !m.HasTable(&enrollmentRow{}) || !m.HasColumn(&authRow{}, "Submicroseconds") {
		return errors.New("incomplete authentication schema")
	}
	for _, model := range applicationModels() {
		if !m.HasTable(model) {
			return errors.New("incomplete application schema")
		}
	}
	_, err := readInstallation(query)
	return err
}

func migrateApplicationTables(tx *gorm.DB) error {
	m := tx.Migrator()
	models := applicationModels()
	for _, model := range models {
		if m.HasTable(model) {
			return errors.New("unversioned application table")
		}
	}
	if !m.HasColumn(&authRow{}, "Submicroseconds") {
		if err := m.AddColumn(&authRow{}, "Submicroseconds"); err != nil {
			return err
		}
	}
	if err := m.CreateTable(models...); err != nil {
		return err
	}
	if err := seedRepositoryLocks(tx); err != nil {
		return err
	}
	if err := seedMessageNotifications(tx); err != nil {
		return err
	}
	if err := seedInstallation(tx); err != nil {
		return err
	}
	return recordSchemaVersion(tx, 2)
}

func recordSchemaVersion(tx *gorm.DB, version int) error {
	return tx.Create(&schemaMigration{Version: version, AppliedAt: time.Now().UTC()}).Error
}

package db

import (
	"context"
	"encoding/base32"
	"errors"
	"regexp"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.TOTPEnrollmentRepository = (*Store)(nil)

var enrollmentIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var ErrStaleEnrollment = errors.New("TOTP enrollment changed or time step already accepted")

func validateEnrollment(r domain.TOTPEnrollmentRecord) error {
	if r.FormatVersion != domain.TOTPFormatVersion || !enrollmentIDPattern.MatchString(r.EnrollmentID) || r.LastAcceptedTimeStep < 0 {
		return errors.New("invalid TOTP enrollment")
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(r.Secret)
	if err != nil || len(secret) < 10 || len(secret) > 128 {
		return errors.New("invalid TOTP enrollment")
	}
	return nil
}

func (s *Store) LoadEnrollment(ctx context.Context) (domain.TOTPEnrollmentRecord, bool, error) {
	var row enrollmentRow
	err := s.DB.WithContext(ctx).Where("key = ?", "primary").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TOTPEnrollmentRecord{}, false, nil
	}
	if err != nil {
		return domain.TOTPEnrollmentRecord{}, false, errors.New("TOTP repository unavailable")
	}
	r := domain.TOTPEnrollmentRecord{FormatVersion: row.FormatVersion, EnrollmentID: row.EnrollmentID,
		Secret: row.Secret, LastAcceptedTimeStep: row.LastAcceptedTimeStep}
	if err := validateEnrollment(r); err != nil {
		return domain.TOTPEnrollmentRecord{}, false, err
	}
	return r, true, nil
}

// SaveEnrollment creates the initial enrollment once, then advances a time step
// only for the same identity and secret. Competing or replayed writes fail closed.
// Reset requires a separate operator operation that also revokes auth bindings.
func (s *Store) SaveEnrollment(ctx context.Context, r domain.TOTPEnrollmentRecord) error {
	if err := validateEnrollment(r); err != nil {
		return err
	}
	row := enrollmentRow{Key: "primary", FormatVersion: r.FormatVersion, EnrollmentID: r.EnrollmentID,
		Secret: r.Secret, LastAcceptedTimeStep: r.LastAcceptedTimeStep}
	created := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if created.Error != nil {
		return errors.New("TOTP repository unavailable")
	}
	if created.RowsAffected == 1 {
		return nil
	}
	updated := s.DB.WithContext(ctx).Model(&enrollmentRow{}).
		Where("key = ? AND format_version = ? AND enrollment_id = ? AND secret = ? AND last_accepted_time_step < ?",
			"primary", r.FormatVersion, r.EnrollmentID, r.Secret, r.LastAcceptedTimeStep).
		Update("last_accepted_time_step", r.LastAcceptedTimeStep)
	if updated.Error != nil {
		return errors.New("TOTP repository unavailable")
	}
	if updated.RowsAffected != 1 {
		return ErrStaleEnrollment
	}
	return nil
}

// ResetEnrollment is an offline administrative operation. Delete the replay
// state and all authentication bindings together or not at all.
func (s *Store) ResetEnrollment(ctx context.Context) error {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockRepository(tx, "auth"); err != nil {
			return err
		}
		if err := tx.Delete(&enrollmentRow{Key: primaryKey}).Error; err != nil {
			return err
		}
		return clearAuth(tx)
	}, repositoryTxOptions(s.DB)...)
	if err != nil {
		return errors.New("TOTP reset failed; database unavailable")
	}
	return nil
}

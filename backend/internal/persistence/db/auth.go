package db

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
)

var _ ports.AuthRepository = (*Store)(nil)

var authHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validAuth(r domain.AuthSessionRecord) bool {
	return enrollmentIDPattern.MatchString(r.ID) && enrollmentIDPattern.MatchString(r.EnrollmentID) &&
		authHashPattern.MatchString(r.PasswordFingerprint) && authHashPattern.MatchString(r.RefreshTokenHash) &&
		!r.CreatedAt.IsZero() && !r.LastSeenAt.IsZero() && !r.RefreshExpiresAt.IsZero() && !r.RotatedAt.IsZero() &&
		r.RefreshExpiresAt.After(r.CreatedAt) && !r.LastSeenAt.Before(r.CreatedAt) &&
		!r.RotatedAt.Before(r.CreatedAt) && utf8.ValidString(r.UserAgent) && len(r.UserAgent) <= 500 && !strings.ContainsRune(r.UserAgent, 0)
}

func (s *Store) LoadAuth(ctx context.Context) ([]domain.AuthSessionRecord, error) {
	var rows []authRow
	if err := s.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, errors.New("authentication repository unavailable")
	}
	result := make([]domain.AuthSessionRecord, 0, len(rows))
	for _, row := range rows {
		r := domain.AuthSessionRecord{ID: row.ID, PasswordFingerprint: row.PasswordFingerprint,
			EnrollmentID: row.EnrollmentID, RefreshTokenHash: row.RefreshTokenHash,
			CreatedAt: row.CreatedAt.UTC(), LastSeenAt: row.LastSeenAt.UTC(),
			RefreshExpiresAt: row.RefreshExpiresAt.UTC(), RotatedAt: row.RotatedAt.UTC(), UserAgent: row.UserAgent}
		if !restoreAuthPrecision(row.Submicroseconds, &r) || !validAuth(r) {
			return nil, errors.New("invalid authentication repository record")
		}
		result = append(result, r)
	}
	return result, nil
}

// SaveAuth implements the existing whole-set port atomically. This does not
// provide cross-process stale-snapshot protection or refresh-token CAS: the
// application must not wire this port for multi-writer traffic as-is.
func (s *Store) SaveAuth(ctx context.Context, records []domain.AuthSessionRecord) error {
	rows := make([]authRow, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, r := range records {
		if !validAuth(r) || seen[r.ID] {
			return errors.New("invalid authentication repository record")
		}
		seen[r.ID] = true
		rows = append(rows, encodeAuthRow(r))
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockRepository(tx, "auth"); err != nil {
			return err
		}
		if len(rows) > 0 {
			var enrollment enrollmentRow
			if err := tx.Take(&enrollment, &enrollmentRow{Key: primaryKey}).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if row.EnrollmentID != enrollment.EnrollmentID {
					return ErrStaleEnrollment
				}
			}
		}
		if err := clearAuth(tx); err != nil || len(rows) == 0 {
			return err
		}
		return tx.CreateInBatches(rows, 100).Error
	}, repositoryTxOptions(s.DB)...)
	if err != nil {
		return errors.New("authentication repository unavailable or enrollment changed")
	}
	return nil
}

func clearAuth(tx *gorm.DB) error {
	return tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&authRow{}).Error
}

// PostgreSQL timestamps store microseconds. Pack the remaining three digits
// per field to retain the exact legacy values in both drivers.
func encodeAuthRow(r domain.AuthSessionRecord) authRow {
	at := [4]time.Time{r.CreatedAt, r.LastSeenAt, r.RefreshExpiresAt, r.RotatedAt}
	var packed int64
	for i := len(at) - 1; i >= 0; i-- {
		packed = packed*1000 + int64(at[i].Nanosecond()%1000)
		at[i] = at[i].UTC().Truncate(time.Microsecond)
	}
	return authRow{ID: r.ID, PasswordFingerprint: r.PasswordFingerprint, EnrollmentID: r.EnrollmentID,
		RefreshTokenHash: r.RefreshTokenHash, UserAgent: r.UserAgent, CreatedAt: at[0], LastSeenAt: at[1],
		RefreshExpiresAt: at[2], RotatedAt: at[3], Submicroseconds: packed}
}

func restoreAuthPrecision(packed int64, r *domain.AuthSessionRecord) bool {
	if packed < 0 || packed >= 1000_000_000_000 {
		return false
	}
	for _, stamp := range []*time.Time{&r.CreatedAt, &r.LastSeenAt, &r.RefreshExpiresAt, &r.RotatedAt} {
		*stamp = stamp.Add(time.Duration(packed % 1000))
		packed /= 1000
	}
	return true
}

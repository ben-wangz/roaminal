package db

import (
	"context"
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ ports.NotificationPreferenceRepository = (*Store)(nil)

var errPreferenceRepository = errors.New("notification preference repository unavailable")

type notificationPreferenceRow struct {
	Position               uint64    `gorm:"primaryKey;autoIncrement"`
	UserKey                string    `gorm:"not null;size:64;uniqueIndex:idx_notification_preference"`
	ConnectionDefinitionID string    `gorm:"not null;size:256;uniqueIndex:idx_notification_preference"`
	TmuxSessionName        string    `gorm:"not null;size:64;uniqueIndex:idx_notification_preference"`
	Enabled                bool      `gorm:"not null"`
	RunningToRelax         bool      `gorm:"not null"`
	RunningToError         bool      `gorm:"not null"`
	Updated                time.Time `gorm:"column:updated_at;type:text;serializer:json;not null"`
}

func (notificationPreferenceRow) TableName() string { return "notification_preferences" }

func newNotificationPreferenceRow(value domain.NotificationPreference) notificationPreferenceRow {
	return notificationPreferenceRow{UserKey: value.UserKey, ConnectionDefinitionID: value.ConnectionDefinitionID,
		TmuxSessionName: value.TmuxSessionName, Enabled: value.Enabled, RunningToRelax: value.RunningToRelax,
		RunningToError: value.RunningToError, Updated: value.UpdatedAt}
}

func (row notificationPreferenceRow) record() domain.NotificationPreference {
	return domain.NotificationPreference{UserKey: row.UserKey, ConnectionDefinitionID: row.ConnectionDefinitionID,
		TmuxSessionName: row.TmuxSessionName, Enabled: row.Enabled, RunningToRelax: row.RunningToRelax,
		RunningToError: row.RunningToError, UpdatedAt: row.Updated}
}

func preferenceKey(userKey, definitionID, tmuxName string) clause.Expression {
	return clause.And(clause.Eq{Column: "user_key", Value: userKey},
		clause.Eq{Column: "connection_definition_id", Value: definitionID},
		clause.Eq{Column: "tmux_session_name", Value: tmuxName})
}

func (s *Store) ListNotificationPreferences(ctx context.Context, userKey string) ([]domain.NotificationPreference, error) {
	ctx = notificationContext(ctx)
	var rows []notificationPreferenceRow
	if err := s.DB.WithContext(ctx).Where(clause.Eq{Column: "user_key", Value: userKey}).Order(originalOrder).Find(&rows).Error; err != nil {
		return nil, notificationDBError(ctx, errPreferenceRepository)
	}
	values := make([]domain.NotificationPreference, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.record())
	}
	return values, nil
}

func (s *Store) GetNotificationPreference(ctx context.Context, userKey, definitionID, tmuxName string) (domain.NotificationPreference, bool, error) {
	ctx = notificationContext(ctx)
	var row notificationPreferenceRow
	err := s.DB.WithContext(ctx).Where(preferenceKey(userKey, definitionID, tmuxName)).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NotificationPreference{}, false, nil
	}
	if err != nil {
		return domain.NotificationPreference{}, false, notificationDBError(ctx, errPreferenceRepository)
	}
	return row.record(), true, nil
}

func (s *Store) UpsertNotificationPreference(ctx context.Context, value domain.NotificationPreference) (domain.NotificationPreference, error) {
	ctx = notificationContext(ctx)
	if err := ctx.Err(); err != nil {
		return domain.NotificationPreference{}, err
	}
	if err := persistence.ValidateNotificationPreference(value); err != nil {
		return domain.NotificationPreference{}, err
	}
	row := newNotificationPreferenceRow(value)
	err := s.writeNotification(ctx, notificationPreferenceLock, errPreferenceRepository, func(tx *gorm.DB) error {
		key := preferenceKey(row.UserKey, row.ConnectionDefinitionID, row.TmuxSessionName)
		var current notificationPreferenceRow
		err := tx.Select("position").Where(key).Take(&current).Error
		switch {
		case err == nil:
			// Explicit fields include false values without moving the row.
			if err := tx.Model(&current).Select("Enabled", "RunningToRelax", "RunningToError", "Updated").Updates(&row).Error; err != nil {
				return errPreferenceRepository
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			var count int64
			if err := tx.Model(&notificationPreferenceRow{}).Count(&count).Error; err != nil {
				return errPreferenceRepository
			}
			if count >= persistence.MaxNotificationPreferences {
				return errors.New("notification preference limit reached")
			}
			if err := tx.Create(&row).Error; err != nil {
				return errPreferenceRepository
			}
		default:
			return errPreferenceRepository
		}
		return nil
	})
	if err != nil {
		return domain.NotificationPreference{}, err
	}
	return value, nil
}

func importNotificationPreferences(tx *gorm.DB, values []domain.NotificationPreference) error {
	if len(values) > persistence.MaxNotificationPreferences {
		return errors.New("notification preference retention limit exceeded")
	}
	if err := lockNotificationRepository(tx, notificationPreferenceLock, errPreferenceRepository); err != nil {
		return err
	}
	if err := requireEmptyRepository(tx, &notificationPreferenceRow{}); err != nil {
		return err
	}
	rows := make([]notificationPreferenceRow, 0, len(values))
	for _, value := range values {
		if err := persistence.ValidateNotificationPreference(value); err != nil {
			return err
		}
		rows = append(rows, newNotificationPreferenceRow(value))
	}
	if len(rows) > 0 && tx.CreateInBatches(&rows, 100).Error != nil {
		return errPreferenceRepository
	}
	return nil
}

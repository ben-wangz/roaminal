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

var _ ports.PushSubscriptionRepository = (*Store)(nil)

var errPushRepository = errors.New("push subscription repository unavailable")

var originalOrder = clause.OrderByColumn{Column: clause.Column{Name: "position"}}

type pushSubscriptionRow struct {
	Position                uint64    `gorm:"primaryKey;autoIncrement"`
	ID                      string    `gorm:"not null;size:36;uniqueIndex"`
	AuthenticationSessionID string    `gorm:"not null;size:36;index"`
	EndpointDigest          []byte    `gorm:"not null;uniqueIndex"`
	Endpoint                []byte    `gorm:"not null"`
	AuthKey                 string    `gorm:"not null"`
	P256dhKey               string    `gorm:"not null"`
	Created                 time.Time `gorm:"column:created_at;type:text;serializer:json;not null"`
	Updated                 time.Time `gorm:"column:updated_at;type:text;serializer:json;not null"`
}

func (pushSubscriptionRow) TableName() string { return "push_subscriptions" }

func newPushSubscriptionRow(record domain.PushSubscriptionRecord) pushSubscriptionRow {
	return pushSubscriptionRow{ID: record.ID, AuthenticationSessionID: record.AuthenticationSessionID,
		EndpointDigest: stringDigest(record.Endpoint), Endpoint: []byte(record.Endpoint), AuthKey: record.AuthKey,
		P256dhKey: record.P256dhKey, Created: record.CreatedAt, Updated: record.UpdatedAt}
}

func (row pushSubscriptionRow) record() domain.PushSubscriptionRecord {
	return domain.PushSubscriptionRecord{ID: row.ID, AuthenticationSessionID: row.AuthenticationSessionID,
		Endpoint: string(row.Endpoint), AuthKey: row.AuthKey, P256dhKey: row.P256dhKey,
		CreatedAt: row.Created, UpdatedAt: row.Updated}
}

func (s *Store) ListPushSubscriptions(ctx context.Context) ([]domain.PushSubscriptionRecord, error) {
	ctx = notificationContext(ctx)
	var rows []pushSubscriptionRow
	if err := s.DB.WithContext(ctx).Order(originalOrder).Find(&rows).Error; err != nil {
		return nil, notificationDBError(ctx, errPushRepository)
	}
	records := make([]domain.PushSubscriptionRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, row.record())
	}
	return records, nil
}

func (s *Store) UpsertPushSubscription(ctx context.Context, record domain.PushSubscriptionRecord) (domain.PushSubscriptionRecord, error) {
	ctx = notificationContext(ctx)
	if err := ctx.Err(); err != nil {
		return domain.PushSubscriptionRecord{}, err
	}
	if err := persistence.ValidatePushSubscription(record); err != nil {
		return domain.PushSubscriptionRecord{}, err
	}
	err := s.writeNotification(ctx, pushSubscriptionLock, errPushRepository, func(tx *gorm.DB) error {
		var current pushSubscriptionRow
		err := tx.Where(clause.Eq{Column: "endpoint_digest", Value: stringDigest(record.Endpoint)}).Take(&current).Error
		switch {
		case err == nil:
			if string(current.Endpoint) != record.Endpoint {
				return errPushRepository
			}
			if current.AuthenticationSessionID == record.AuthenticationSessionID {
				record.ID = current.ID
				record.CreatedAt = current.Created
				if record.UpdatedAt.Before(record.CreatedAt) {
					return errors.New("push subscription timestamps are invalid")
				}
			}
			// A refresh moves the endpoint to the end of the list for both
			// the current owner and a replacement owner, as in the file.
			if err := tx.Delete(&current).Error; err != nil {
				return errPushRepository
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			var count int64
			if err := tx.Model(&pushSubscriptionRow{}).Count(&count).Error; err != nil {
				return errPushRepository
			}
			if count >= persistence.MaxPushSubscriptions {
				return errors.New("push subscription limit reached")
			}
		default:
			return errPushRepository
		}
		row := newPushSubscriptionRow(record)
		if err := tx.Create(&row).Error; err != nil {
			return errPushRepository
		}
		return nil
	})
	if err != nil {
		return domain.PushSubscriptionRecord{}, err
	}
	return record, nil
}

func (s *Store) DeletePushSubscription(ctx context.Context, authSessionID, subscriptionID string) (bool, error) {
	count, err := s.deletePushSubscriptions(ctx, clause.And(
		clause.Eq{Column: "id", Value: subscriptionID},
		clause.Eq{Column: "authentication_session_id", Value: authSessionID},
	))
	return count > 0, err
}

func (s *Store) DeletePushSubscriptionsForAuthSession(ctx context.Context, authSessionID string) (int, error) {
	return s.deletePushSubscriptions(ctx, clause.Eq{Column: "authentication_session_id", Value: authSessionID})
}

func (s *Store) DeletePushSubscriptionByID(ctx context.Context, subscriptionID string) (bool, error) {
	count, err := s.deletePushSubscriptions(ctx, clause.Eq{Column: "id", Value: subscriptionID})
	return count > 0, err
}

func (s *Store) deletePushSubscriptions(ctx context.Context, predicate clause.Expression) (int, error) {
	var deleted int
	err := s.writeNotification(notificationContext(ctx), pushSubscriptionLock, errPushRepository, func(tx *gorm.DB) error {
		result := tx.Where(predicate).Delete(&pushSubscriptionRow{})
		if result.Error != nil {
			return errPushRepository
		}
		deleted = int(result.RowsAffected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func importPushSubscriptions(tx *gorm.DB, records []domain.PushSubscriptionRecord) error {
	if len(records) > persistence.MaxPushSubscriptions {
		return errors.New("push subscription retention limit exceeded")
	}
	if err := lockNotificationRepository(tx, pushSubscriptionLock, errPushRepository); err != nil {
		return err
	}
	if err := requireEmptyRepository(tx, &pushSubscriptionRow{}); err != nil {
		return err
	}
	rows := make([]pushSubscriptionRow, 0, len(records))
	for _, record := range records {
		if err := persistence.ValidatePushSubscription(record); err != nil {
			return err
		}
		rows = append(rows, newPushSubscriptionRow(record))
	}
	if len(rows) > 0 && tx.CreateInBatches(&rows, 100).Error != nil {
		return errPushRepository
	}
	return nil
}

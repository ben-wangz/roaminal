package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	pushSubscriptionLock       = "push_subscriptions"
	notificationPreferenceLock = "notification_preferences"
)

func messageNotificationModels() []any {
	return []any{&messageStateRow{}, &messageRow{}, &pushSubscriptionRow{}, &notificationPreferenceRow{}}
}

// seedMessageNotifications runs once in the migration creating these tables.
func seedMessageNotifications(tx *gorm.DB) error {
	return tx.Create(&messageStateRow{ID: messageStateID}).Error
}

// initializeMessageNotifications checks the seeded singleton records and
// mirrors startup age pruning when the store is prepared to serve.
func (s *Store) initializeMessageNotifications(tx *gorm.DB) error {
	state, err := lockMessageState(tx)
	if err != nil {
		return err
	}
	var locks int64
	if err := tx.Model(&repositoryLock{}).Where(clause.IN{Column: "key", Values: []any{pushSubscriptionLock, notificationPreferenceLock}}).Count(&locks).Error; err != nil || locks != 2 {
		return errors.New("notification repository locks unavailable")
	}
	pruned, err := pruneExpiredMessages(tx, time.Now().UTC().Add(-persistence.MessageMaxAge))
	if err != nil {
		return err
	}
	if pruned {
		return reviseMessageState(tx, &state)
	}
	return nil
}

// Indexed digests avoid PostgreSQL's text NUL restriction and B-tree key
// size limit. Repositories still compare the actual value before reuse.
func stringDigest(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func repositoryTxOptions(db *gorm.DB) []*sql.TxOptions {
	if db.Dialector.Name() == Postgres {
		// After waiting for a row lock, the dependent reads must see the
		// preceding writer, regardless of the database's default isolation.
		return []*sql.TxOptions{{Isolation: sql.LevelReadCommitted}}
	}
	return nil
}

func lockNotificationRepository(tx *gorm.DB, repository string, unavailable error) error {
	if err := lockRepository(tx, repository); err != nil {
		return unavailable
	}
	return nil
}

func notificationContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func notificationDBError(ctx context.Context, unavailable error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return unavailable
}

func (s *Store) writeNotification(ctx context.Context, repository string, unavailable error, run func(*gorm.DB) error) error {
	var operationErr error
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockNotificationRepository(tx, repository, unavailable); err != nil {
			return err
		}
		operationErr = run(tx)
		return operationErr
	}, repositoryTxOptions(s.DB)...)
	if err != nil {
		if operationErr != nil && ctx.Err() == nil {
			return operationErr
		}
		return notificationDBError(ctx, unavailable)
	}
	return nil
}

func requireEmptyRepository(tx *gorm.DB, model any) error {
	var count int64
	if err := tx.Model(model).Count(&count).Error; err != nil {
		return errors.New("import destination unavailable")
	}
	if count != 0 {
		return errors.New("import destination is not empty")
	}
	return nil
}

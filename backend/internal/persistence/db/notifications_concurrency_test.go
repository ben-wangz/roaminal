package db

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"gorm.io/gorm"
)

func TestSQLitePushSubscriptionsConcurrentCapacity(t *testing.T) {
	s, c := sqliteStore(t)
	stores := []*Store{s, reopenMessageNotificationStore(t, c)}
	seed := pushFixture(t, 1)
	const limit = persistence.MaxPushSubscriptions
	records := make([]domain.PushSubscriptionRecord, limit-1)
	for i := range records {
		records[i] = seed
		records[i].ID = notificationTestID(i + 1)
		records[i].Endpoint = fmt.Sprintf("https://push.example.test/%d", i+1)
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error { return importPushSubscriptions(tx, records) }); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	candidates := []domain.PushSubscriptionRecord{pushFixture(t, limit), pushFixture(t, limit+1)}
	checkLastNotificationSlot(t, "push subscription limit reached", func(i int) error {
		_, err := stores[i].UpsertPushSubscription(ctx, candidates[i])
		return err
	})
	if _, err := s.UpsertPushSubscription(ctx, seed); err != nil {
		t.Fatalf("refresh at subscription limit: %v", err)
	}
	got, err := s.ListPushSubscriptions(ctx)
	if err != nil || len(got) != limit || got[limit-1].ID != seed.ID {
		t.Fatalf("subscriptions at limit: count = %d, err = %v", len(got), err)
	}
}

func TestSQLiteNotificationPreferencesConcurrentCapacity(t *testing.T) {
	s, c := sqliteStore(t)
	stores := []*Store{s, reopenMessageNotificationStore(t, c)}
	const limit = persistence.MaxNotificationPreferences
	values := make([]domain.NotificationPreference, limit-1)
	for i := range values {
		values[i] = preferenceFixture(i + 1)
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error { return importNotificationPreferences(tx, values) }); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	checkLastNotificationSlot(t, "notification preference limit reached", func(i int) error {
		_, err := stores[i].UpsertNotificationPreference(ctx, preferenceFixture(limit+i))
		return err
	})
	replace := values[0]
	replace.Enabled = false
	if _, err := s.UpsertNotificationPreference(ctx, replace); err != nil {
		t.Fatalf("replace at preference limit: %v", err)
	}
	got, err := s.ListNotificationPreferences(ctx, replace.UserKey)
	if err != nil || len(got) != limit || got[0] != replace {
		t.Fatalf("preferences at limit: count = %d, err = %v", len(got), err)
	}
}

func checkLastNotificationSlot(t *testing.T, limitMessage string, insert func(int) error) {
	t.Helper()
	var succeeded atomic.Int32
	runConcurrentRepositoryCalls(2, func(i int) {
		if err := insert(i); err == nil {
			succeeded.Add(1)
		} else if err.Error() != limitMessage {
			t.Errorf("concurrent insert: err = %v, want %q", err, limitMessage)
		}
	})
	if got := succeeded.Load(); got != 1 {
		t.Fatalf("successful inserts for one remaining slot = %d", got)
	}
}

package db

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"gorm.io/gorm"
)

func notificationTestID(i int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012x", i) }

func pushFixture(t *testing.T, i int) domain.PushSubscriptionRecord {
	t.Helper()
	private := make([]byte, 32)
	private[31] = 1
	key, err := ecdh.P256().NewPrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second).Add(123456789)
	return domain.PushSubscriptionRecord{ID: notificationTestID(i), AuthenticationSessionID: notificationTestID(50000),
		Endpoint: fmt.Sprintf("https://push.example.test/%d", i), AuthKey: base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
		P256dhKey: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), CreatedAt: now, UpdatedAt: now}
}

func TestSQLitePushSubscriptionRepository(t *testing.T) {
	s, c := sqliteStore(t)
	testPushSubscriptionRepository(t, s)
	s = reopenMessageNotificationStore(t, c)
	records, err := s.ListPushSubscriptions(context.Background())
	if err != nil || len(records) != 1 || records[0].ID != notificationTestID(4) || records[0].AuthenticationSessionID != notificationTestID(50001) {
		t.Fatalf("reopened subscription count = %d, err = %v", len(records), err)
	}
}

func testPushSubscriptionRepository(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	a, b := pushFixture(t, 1), pushFixture(t, 2)
	a.Endpoint += strings.Repeat("~", 3500)
	secondOwner := notificationTestID(50001)
	b.AuthenticationSessionID = secondOwner
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		return importPushSubscriptions(tx, []domain.PushSubscriptionRecord{a, b})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		return importPushSubscriptions(tx, []domain.PushSubscriptionRecord{a})
	}); err == nil {
		t.Fatal("overwrote existing subscriptions on import")
	}
	retry := a
	retry.ID = notificationTestID(3)
	retry.CreatedAt = a.CreatedAt.Add(time.Minute)
	retry.UpdatedAt = retry.CreatedAt.Add(time.Minute)
	got, err := s.UpsertPushSubscription(ctx, retry)
	if err != nil || got.ID != a.ID || !got.CreatedAt.Equal(a.CreatedAt) || !got.UpdatedAt.Equal(retry.UpdatedAt) {
		t.Fatalf("same-owner refresh id = %q, err = %v", got.ID, err)
	}
	values, err := s.ListPushSubscriptions(ctx)
	if err != nil || len(values) != 2 || values[0] != b || values[1] != got {
		t.Fatalf("refresh order or private record lost: count = %d, err = %v", len(values), err)
	}
	retry.UpdatedAt = a.CreatedAt.Add(-time.Second)
	retry.CreatedAt = retry.UpdatedAt
	if _, err := s.UpsertPushSubscription(ctx, retry); err == nil {
		t.Fatal("accepted refresh before preserved creation timestamp")
	}
	retry.CreatedAt, retry.UpdatedAt = a.CreatedAt.Add(time.Hour), a.CreatedAt.Add(time.Hour)
	retry.AuthenticationSessionID = secondOwner
	retry.ID = b.ID
	if _, err := s.UpsertPushSubscription(ctx, retry); err == nil {
		t.Fatal("reused an ID from a different endpoint")
	}
	values, err = s.ListPushSubscriptions(ctx)
	if err != nil || len(values) != 2 || values[1] != got {
		t.Fatalf("failed refresh changed records: count = %d, err = %v", len(values), err)
	}
	retry.ID = notificationTestID(4)
	got, err = s.UpsertPushSubscription(ctx, retry)
	if err != nil || got != retry {
		t.Fatalf("transfer id = %q, err = %v", got.ID, err)
	}
	if deleted, err := s.DeletePushSubscription(ctx, a.AuthenticationSessionID, got.ID); err != nil || deleted {
		t.Fatalf("previous owner delete = %v, %v", deleted, err)
	}
	if deleted, err := s.DeletePushSubscriptionsForAuthSession(ctx, a.AuthenticationSessionID); err != nil || deleted != 0 {
		t.Fatalf("previous owner cleanup = %v, %v", deleted, err)
	}
	if deleted, err := s.DeletePushSubscription(ctx, secondOwner, b.ID); err != nil || !deleted {
		t.Fatalf("owner delete = %v, %v", deleted, err)
	}
	for i := 5; i <= 7; i++ {
		if _, err := s.UpsertPushSubscription(ctx, pushFixture(t, i)); err != nil {
			t.Fatal(err)
		}
	}
	if deleted, err := s.DeletePushSubscriptionByID(ctx, notificationTestID(7)); err != nil || !deleted {
		t.Fatalf("expired endpoint cleanup = %v, %v", deleted, err)
	}
	if deleted, err := s.DeletePushSubscriptionsForAuthSession(ctx, a.AuthenticationSessionID); err != nil || deleted != 2 {
		t.Fatalf("owner cleanup = %v, %v", deleted, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.DeletePushSubscriptionByID(canceled, got.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delete = %v", err)
	}
	bad := retry
	bad.AuthKey = "invalid"
	if _, err := s.UpsertPushSubscription(ctx, bad); err == nil {
		t.Fatal("invalid push key accepted")
	}
}

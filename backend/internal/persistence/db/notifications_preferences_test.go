package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"gorm.io/gorm"
)

func preferenceFixture(i int) domain.NotificationPreference {
	return domain.NotificationPreference{UserKey: strings.Repeat("a", 64), ConnectionDefinitionID: fmt.Sprintf("definition-%d", i),
		TmuxSessionName: "team", Enabled: true, RunningToRelax: true, RunningToError: true,
		UpdatedAt: time.Now().UTC().Truncate(time.Second).Add(987654321)}
}

func TestSQLiteNotificationPreferenceRepository(t *testing.T) {
	s, c := sqliteStore(t)
	testNotificationPreferenceRepository(t, s)
	s = reopenMessageNotificationStore(t, c)
	seed := preferenceFixture(1)
	got, exists, err := s.GetNotificationPreference(context.Background(), seed.UserKey, seed.ConnectionDefinitionID, seed.TmuxSessionName)
	if err != nil || !exists || got.Enabled || got.RunningToError || got.RunningToRelax || got.UpdatedAt.Nanosecond() != seed.UpdatedAt.Nanosecond() {
		t.Fatalf("reopened preference exists = %t, err = %v", exists, err)
	}
}

func testNotificationPreferenceRepository(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	a, b := preferenceFixture(1), preferenceFixture(2)
	otherUser := a
	otherUser.UserKey = strings.Repeat("b", 64)
	otherTmux := a
	otherTmux.TmuxSessionName = "ops"
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		return importNotificationPreferences(tx, []domain.NotificationPreference{a, b, otherUser, otherTmux})
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		return importNotificationPreferences(tx, []domain.NotificationPreference{a})
	}); err == nil {
		t.Fatal("overwrote existing preferences on import")
	}
	a.Enabled, a.RunningToRelax, a.RunningToError = false, false, false
	a.UpdatedAt = a.UpdatedAt.Add(time.Minute)
	if got, err := s.UpsertNotificationPreference(ctx, a); err != nil || got != a {
		t.Fatalf("replace notification flags: err = %v", err)
	}
	values, err := s.ListNotificationPreferences(ctx, a.UserKey)
	if err != nil || len(values) != 3 || values[0] != a || values[1] != b || values[2] != otherTmux {
		t.Fatalf("preference list order or contents lost: count = %d, err = %v", len(values), err)
	}
	got, exists, err := s.GetNotificationPreference(ctx, otherUser.UserKey, a.ConnectionDefinitionID, a.TmuxSessionName)
	if err != nil || !exists || got != otherUser {
		t.Fatalf("second user's preference changed, exists = %t, err = %v", exists, err)
	}
	got, exists, err = s.GetNotificationPreference(ctx, a.UserKey, a.ConnectionDefinitionID, "")
	if err != nil || exists || got != (domain.NotificationPreference{}) {
		t.Fatalf("missing exact key: exists = %t, err = %v", exists, err)
	}
	if values, err := s.ListNotificationPreferences(ctx, ""); err != nil || values == nil || len(values) != 0 {
		t.Fatalf("empty list: count = %d, err = %v", len(values), err)
	}
	bad := a
	bad.TmuxSessionName = "bad name"
	if _, err := s.UpsertNotificationPreference(ctx, bad); err == nil {
		t.Fatal("invalid preference accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ListNotificationPreferences(canceled, a.UserKey); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled list = %v", err)
	}
	if _, _, err := s.GetNotificationPreference(canceled, a.UserKey, a.ConnectionDefinitionID, a.TmuxSessionName); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled get = %v", err)
	}
}

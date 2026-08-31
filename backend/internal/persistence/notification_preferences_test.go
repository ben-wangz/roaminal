package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestNotificationPreferencesPersistReplaceAndIsolateUsers(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).NotificationPreferences
	now := time.Now().UTC()
	first := domain.NotificationPreference{
		UserKey:                "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ConnectionDefinitionID: "definition-a", TmuxSessionName: "team",
		Enabled: true, RunningToRelax: true, UpdatedAt: now,
	}
	second := domain.NotificationPreference{
		UserKey:                "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ConnectionDefinitionID: "definition-a", TmuxSessionName: "team",
		Enabled: false, RunningToError: true, UpdatedAt: now,
	}
	if _, err := repository.UpsertNotificationPreference(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertNotificationPreference(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	first.Enabled = false
	first.RunningToError = true
	first.UpdatedAt = now.Add(time.Minute)
	if _, err := repository.UpsertNotificationPreference(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	values, err := repository.ListNotificationPreferences(context.Background(), first.UserKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Enabled || !values[0].RunningToError {
		t.Fatalf("unexpected replaced preferences: %+v", values)
	}
	if _, ok, err := repository.GetNotificationPreference(context.Background(), second.UserKey, second.ConnectionDefinitionID, second.TmuxSessionName); err != nil || !ok {
		t.Fatalf("second user's preference missing: ok=%v err=%v", ok, err)
	}

	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	values, err = NewRepositories(reopened).NotificationPreferences.ListNotificationPreferences(context.Background(), first.UserKey)
	if err != nil || len(values) != 1 || values[0].UpdatedAt.Before(first.UpdatedAt) {
		t.Fatalf("preferences did not survive reopen: values=%+v err=%v", values, err)
	}
	info, err := os.Stat(filepath.Join(root, "notification-preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("preference file mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestNotificationPreferencesRejectInvalidRecords(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).NotificationPreferences
	base := domain.NotificationPreference{
		UserKey:                "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ConnectionDefinitionID: "definition-a", TmuxSessionName: "team", UpdatedAt: time.Now().UTC(),
	}
	for _, value := range []domain.NotificationPreference{
		base,
		{UserKey: base.UserKey, ConnectionDefinitionID: "definition-a", TmuxSessionName: "bad name", UpdatedAt: base.UpdatedAt},
		{UserKey: base.UserKey, ConnectionDefinitionID: "", TmuxSessionName: "team", UpdatedAt: base.UpdatedAt},
		{UserKey: base.UserKey, ConnectionDefinitionID: "definition-a", TmuxSessionName: "team", UpdatedAt: time.Time{}},
	} {
		if value == base {
			value.UserKey = "short"
		}
		if _, err := repository.UpsertNotificationPreference(context.Background(), value); err == nil {
			t.Fatalf("invalid preference was accepted: %+v", value)
		}
	}
	if _, err := repository.UpsertNotificationPreference(context.Background(), base); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationPreferencesHonorContextCancellation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).NotificationPreferences
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = repository.ListNotificationPreferences(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled list error = %v", err)
	}
}

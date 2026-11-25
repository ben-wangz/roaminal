package db

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

const (
	firstInstance  domain.ConnectionInstanceID = "10000000-0000-4000-8000-000000000000"
	secondInstance domain.ConnectionInstanceID = "20000000-0000-4000-8000-000000000000"
	thirdInstance  domain.ConnectionInstanceID = "30000000-0000-4000-8000-000000000000"
)

var instanceTime = time.Date(2026, time.March, 1, 12, 30, 0, 123456789, time.UTC)

func instanceFixture(id domain.ConnectionInstanceID) domain.ConnectionInstanceMeta {
	return domain.ConnectionInstanceMeta{
		ID: id.String(), BackendRuntimeID: "runtime", ConnectionDefinitionID: "local",
		Type: "local", Purpose: "interactive", Lifecycle: "live", SourceState: "current",
		InitialCwd: "/workspace", Cwd: "/workspace/project", Cols: 80, Rows: 24,
		TerminalType: "xterm-256color", CreatedAt: instanceTime, UpdatedAt: instanceTime,
	}
}

func saveInstanceFixture(t *testing.T, s *Store, meta domain.ConnectionInstanceMeta) {
	t.Helper()
	if err := s.SaveConnectionInstance(context.Background(), meta); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteInstanceMetadata(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	if got, err := s.ListConnectionInstances(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("fresh list = %v, %v", got, err)
	}
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) || s.PersistenceDegraded() {
		t.Fatalf("missing lookup: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	meta := instanceFixture(secondInstance)
	meta.Title = "initial title"
	saveInstanceFixture(t, s, meta)
	got, err := s.LoadConnectionInstance(ctx, secondInstance)
	want := meta
	want.AutomaticTitle = meta.Title
	want.FormatVersion = persistence.ConnectionFormatVersion
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("initial metadata = %+v, %v; want %+v", got, err, want)
	}

	alias, override, signal, sourceID := "host", "custom title", "9", firstInstance.String()
	exitCode := 1
	want.Title, want.TitleOverride = override, &override
	want.Type, want.ConnectionDefinitionID, want.SourceHostAlias = "ssh", "ssh:host", &alias
	want.TmuxEnabled, want.TmuxSessionName = true, "project"
	want.TmuxPrefixKey, want.TmuxPrefixSource = "a", "runtime"
	want.ExitCode, want.ExitSignal, want.ReuseFromConnectionInstanceID = &exitCode, &signal, &sourceID
	want.Lifecycle, want.UpdatedAt = "exited", instanceTime.Add(time.Second)
	saveInstanceFixture(t, s, want)
	got, err = s.LoadConnectionInstance(ctx, secondInstance)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("updated metadata = %+v, %v; want %+v", got, err, want)
	}

	saveInstanceFixture(t, s, instanceFixture(firstInstance))
	earlier := instanceFixture(thirdInstance)
	earlier.CreatedAt = earlier.CreatedAt.Add(-time.Nanosecond)
	saveInstanceFixture(t, s, earlier)
	list, err := s.ListConnectionInstances(ctx)
	if err != nil || len(list) != 3 || list[0].ID != earlier.ID || list[1].ID != firstInstance.String() || list[2].ID != want.ID {
		t.Fatalf("list not ordered by creation time then id: %+v, %v", list, err)
	}
}

func TestSQLiteCorruptInstanceMetadataIsNotListed(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	saveInstanceFixture(t, s, instanceFixture(firstInstance))
	saveInstanceFixture(t, s, instanceFixture(secondInstance))
	data, err := encodeInstanceMetadata(instanceFixture(secondInstance))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Model(&instanceRow{}).Where("id = ?", firstInstance.String()).Update("metadata", data).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); !errors.Is(err, errInstanceMetadata) {
		t.Fatalf("accepted mismatched metadata: %v", err)
	}
	list, err := s.ListConnectionInstances(ctx)
	if err != nil || len(list) != 1 || list[0].ID != secondInstance.String() || !s.PersistenceDegraded() {
		t.Fatalf("list after corrupt record = %+v, %v; degraded=%v", list, err, s.PersistenceDegraded())
	}
	saveInstanceFixture(t, s, instanceFixture(firstInstance))
	if s.PersistenceDegraded() {
		t.Fatal("replacing the corrupt metadata did not clear degradation")
	}
}

func TestSQLiteInstanceDegradationAndCancellation(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	bad := instanceFixture(firstInstance)
	bad.Cwd = "relative"
	if err := s.SaveConnectionInstance(ctx, bad); err == nil || !s.PersistenceDegraded() {
		t.Fatalf("invalid metadata: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid metadata was written: %v", err)
	}
	if err := s.MarkConnectionInstanceDegraded(ctx, secondInstance); err != nil {
		t.Fatal(err)
	}
	saveInstanceFixture(t, s, instanceFixture(firstInstance))
	if !s.PersistenceDegraded() {
		t.Fatal("the other instance's failure was cleared")
	}
	if err := s.DeleteConnectionInstance(ctx, secondInstance); err != nil || s.PersistenceDegraded() {
		t.Fatalf("idempotent deletion did not clear degradation: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.DeleteConnectionInstance(canceled, firstInstance); !errors.Is(err, context.Canceled) || s.PersistenceDegraded() {
		t.Fatalf("canceled deletion: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatalf("canceled deletion modified the live record: %v", err)
	}
}

func TestSQLiteInstanceListFailureAndRecovery(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	if err := s.DB.Migrator().DropTable(&instanceRow{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListConnectionInstances(ctx); !errors.Is(err, errInstanceDatabase) || !s.PersistenceDegraded() {
		t.Fatalf("missing instance table: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	if err := s.DB.Migrator().CreateTable(&instanceRow{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListConnectionInstances(ctx); err != nil || s.PersistenceDegraded() {
		t.Fatalf("recovered instance table: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
}

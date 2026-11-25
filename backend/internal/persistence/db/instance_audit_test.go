package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"gorm.io/gorm"
)

func findAudit(t *testing.T, s *Store, id domain.ConnectionInstanceID) instanceAuditRow {
	t.Helper()
	var row instanceAuditRow
	if err := s.DB.Where("id = ?", id.String()).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

func checkAudit(t *testing.T, row instanceAuditRow, title string, snapshot *string) {
	t.Helper()
	meta, err := decodeInstanceMetadata(instanceRow{ID: row.ID, Metadata: row.Metadata})
	if err != nil || meta.Title != title {
		t.Fatalf("audit metadata = %+v, %v", meta, err)
	}
	if snapshot == nil {
		if row.Snapshot != nil {
			t.Fatal("audit contained an unexpected snapshot")
		}
		return
	}
	_, payload, err := persistence.DecodeSnapshot(row.Snapshot)
	if err != nil || string(payload) != *snapshot {
		t.Fatalf("audit snapshot = %q, %v", payload, err)
	}
}

func testInstanceArchiveContract(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	meta := instanceFixture(firstInstance)
	meta.AutomaticTitle = "first checkpoint"
	saveInstanceFixture(t, s, meta)
	if err := s.ArchiveConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatal(err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), meta.AutomaticTitle, nil)
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatalf("archiving removed active metadata: %v", err)
	}

	meta.AutomaticTitle, meta.Lifecycle = "final checkpoint", "exited"
	meta.UpdatedAt = instanceTime.Add(time.Second)
	saveInstanceFixture(t, s, meta)
	header, payload := snapshotFixture(), "terminal output\r\n"
	if err := s.SaveSnapshot(ctx, firstInstance, header, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatalf("retirement retry: %v", err)
	}
	if err := s.DeleteConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatalf("active deletion retry: %v", err)
	}
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired metadata lookup: %v", err)
	}
	if _, _, err := s.LoadSnapshot(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired snapshot lookup: %v", err)
	}
	if list, err := s.ListConnectionInstances(ctx); err != nil || len(list) != 0 {
		t.Fatalf("audit record in active listing: %v, %v", list, err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), meta.AutomaticTitle, &payload)
}

func TestSQLiteInstanceAuditIsDurable(t *testing.T) {
	s, c := sqliteStore(t)
	testInstanceArchiveContract(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	payload := "terminal output\r\n"
	checkAudit(t, findAudit(t, reopened, firstInstance), "final checkpoint", &payload)
}

func TestSQLiteInstanceArchiveRetainsPriorSnapshot(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	meta := instanceFixture(firstInstance)
	header, payload := snapshotFixture(), "earlier snapshot"
	if err := s.ImportArchivedConnectionInstance(ctx, meta, &header, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	meta.Title = "latest metadata, no active snapshot"
	saveInstanceFixture(t, s, meta)
	if err := s.RetireConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatal(err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), meta.Title, &payload)
}

func TestSQLiteInstanceRetirementRollsBackFailedDeletion(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	meta := instanceFixture(firstInstance)
	meta.Title = "old archive"
	saveInstanceFixture(t, s, meta)
	if err := s.ArchiveConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatal(err)
	}
	meta.Title = "new active metadata"
	saveInstanceFixture(t, s, meta)
	if err := s.SaveSnapshot(ctx, firstInstance, snapshotFixture(), []byte("new output")); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Exec(`CREATE TRIGGER prevent_instance_delete BEFORE DELETE ON connection_instances
		BEGIN SELECT RAISE(ABORT, 'private driver details'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RetireConnectionInstance(ctx, firstInstance); !errors.Is(err, errInstanceDatabase) || !s.PersistenceDegraded() {
		t.Fatalf("retirement did not report failed deletion: %v", err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), "old archive", nil)
	if got, err := s.LoadConnectionInstance(ctx, firstInstance); err != nil || got.Title != meta.Title {
		t.Fatalf("active metadata lost on rollback: %+v, %v", got, err)
	}
	if _, payload, err := s.LoadSnapshot(ctx, firstInstance); err != nil || string(payload) != "new output" {
		t.Fatalf("active snapshot lost on rollback: %q, %v", payload, err)
	}
	if err := s.DB.Exec("DROP TRIGGER prevent_instance_delete").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RetireConnectionInstance(ctx, firstInstance); err != nil || s.PersistenceDegraded() {
		t.Fatalf("retirement recovery: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	payload := "new output"
	checkAudit(t, findAudit(t, s, firstInstance), meta.Title, &payload)
}

func TestSQLiteArchivedInstanceImportJoinsTransaction(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	meta := instanceFixture(firstInstance)
	meta.Title = "archived import"
	header := snapshotFixture()
	stop := errors.New("abort import")
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		txStore := &Store{DB: tx, driver: s.driver}
		if err := txStore.SaveConnectionInstance(ctx, instanceFixture(secondInstance)); err != nil {
			return err
		}
		if err := txStore.SaveSnapshot(ctx, secondInstance, header, []byte("active")); err != nil {
			return err
		}
		if err := txStore.ImportArchivedConnectionInstance(ctx, meta, &header, nil); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("import transaction: %v", err)
	}
	if _, err := s.LoadConnectionInstance(ctx, secondInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back import left active metadata: %v", err)
	}
	if _, _, err := s.LoadSnapshot(ctx, secondInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back import left active snapshot: %v", err)
	}
	if err := s.DB.Where("id = ?", meta.ID).Take(&instanceAuditRow{}).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("rolled-back import left audit metadata: %v", err)
	}

	if err := s.ImportArchivedConnectionInstance(ctx, meta, &header, nil); err != nil {
		t.Fatal(err)
	}
	empty := ""
	checkAudit(t, findAudit(t, s, firstInstance), meta.Title, &empty)
	if _, err := s.LoadConnectionInstance(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audit import wrote an active instance: %v", err)
	}
	if err := s.ImportArchivedConnectionInstance(ctx, meta, nil, nil); err != nil {
		t.Fatal(err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), meta.Title, nil)
}

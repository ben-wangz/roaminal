package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

func TestOfflineActivationRejectsChangedRowsUntilFirstStart(t *testing.T) {
	stateDir, source := sqliteRuntime(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, source))
	binding := readRuntimeBinding(t, stateDir)
	seedRuntimeAuth(t, source)
	backup := filepath.Join(t.TempDir(), "source-transfer")
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "export", "--output", backup))

	target := sqliteAt(filepath.Join(t.TempDir(), "destination"))
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", target.SQLitePath)
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import", "--input", backup))
	restored := openRuntimeStore(t, target, false)
	record, exists, err := restored.LoadEnrollment(t.Context())
	requireRuntimeSuccess(t, err)
	if !exists {
		t.Fatal("fixture did not restore enrollment")
	}
	sessions, err := restored.LoadAuth(t.Context())
	requireRuntimeSuccess(t, err)
	deleteRows := func() { requireRuntimeSuccess(t, restored.ResetEnrollment(t.Context())) }
	restoreRows := func() {
		requireRuntimeSuccess(t, restored.SaveEnrollment(t.Context(), record))
		requireRuntimeSuccess(t, restored.SaveAuth(t.Context(), sessions))
	}

	deleteRows()
	if err := runtimeMaintenance(t, "db", "import", "--input", backup); !errors.Is(err, db.ErrTransferVerification) {
		t.Fatalf("reimport accepted missing rows: %v", err)
	}
	if err := runtimeMaintenance(t, "db", "activate"); !errors.Is(err, db.ErrTransferVerification) {
		t.Fatalf("activated a database with missing rows: %v", err)
	}
	assertRuntimeBinding(t, stateDir, binding)
	fresh := filepath.Join(t.TempDir(), "fresh")
	if err := startDatabase(t, fresh, target); !errors.Is(err, db.ErrTransferVerification) {
		t.Fatalf("fresh state accepted missing rows: %v", err)
	}
	assertRuntimeBinding(t, fresh, databaseBinding{})

	restoreRows()
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "activate"))
	activated := readRuntimeBinding(t, stateDir)
	deleteRows()
	if err := runtimeMaintenance(t, "db", "activate"); !errors.Is(err, db.ErrTransferVerification) {
		t.Fatalf("activation retry accepted missing rows before first startup: %v", err)
	}
	if err := startDatabase(t, stateDir, target); !errors.Is(err, db.ErrTransferVerification) {
		t.Fatalf("first startup accepted missing rows after activation: %v", err)
	}
	assertRuntimeBinding(t, stateDir, activated)
	if readRuntimeInstallation(t, target).Served {
		t.Fatal("damaged database marked served")
	}
	restoreRows()
	requireRuntimeSuccess(t, startDatabase(t, stateDir, target))
	assertRuntimeAuth(t, target, true)
	// Once served, legitimate application changes no longer match the backup.
	deleteRows()
	requireRuntimeSuccess(t, startDatabase(t, stateDir, target))
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "activate"))
}

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

func TestOfflineActivationRequiresVerifiedTransfer(t *testing.T) {
	stateDir, src := sqliteRuntime(t)
	legacy := putRuntimeLegacyFiles(t, stateDir)
	importRuntimeLegacyFiles(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, src))
	original := readRuntimeBinding(t, stateDir)
	backup := filepath.Join(t.TempDir(), "transfer")
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "export", "--output", backup))
	requireRuntimeSuccess(t, os.WriteFile(legacy, []byte("invalid stale legacy options"), 0o600))

	target := sqliteAt(filepath.Join(t.TempDir(), "destination"))
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", target.SQLitePath)
	if err := runtimeMaintenance(t, "db", "activate"); err == nil {
		t.Fatal("activated a missing target")
	}
	if _, err := os.Lstat(target.SQLitePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("activation created a database: %v", err)
	}

	corrupt := readRuntimeFile(t, backup)
	corrupt[len(corrupt)-1] ^= 1
	badBackup := filepath.Join(t.TempDir(), "corrupt-transfer")
	requireRuntimeSuccess(t, os.WriteFile(badBackup, corrupt, 0o600))
	if err := runtimeMaintenance(t, "db", "import", "--input", badBackup); err == nil {
		t.Fatal("accepted a damaged export")
	}
	if err := runtimeMaintenance(t, "db", "activate"); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("activated an unverified target: %v", err)
	}
	assertRuntimeBinding(t, stateDir, original)

	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import", "--input", backup))
	if err := startDatabase(t, stateDir, target); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("startup without activation: %v", err)
	}
	before := readRuntimeInstallation(t, target)
	if before.Served || before.TransferSourceID != original.ID {
		t.Fatal("import or rejected startup incorrectly claimed the destination")
	}
	locked, err := lockStateDir(stateDir)
	requireRuntimeSuccess(t, err)
	err = runtimeMaintenance(t, "db", "activate")
	locked.close()
	if !errors.Is(err, errStateLocked) {
		t.Fatalf("activation while runtime owns the state lock: %v", err)
	}
	assertRuntimeBinding(t, stateDir, original)

	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "activate"))
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "activate"))
	assertRuntimeBinding(t, stateDir, databaseBinding{
		Driver: db.SQLite, ID: before.ID, Owner: original.Owner, TransferSourceID: original.ID,
	})
	if got := readRuntimeInstallation(t, target); got != before {
		t.Fatal("activation mutated the destination instead of updating the local binding")
	}
	if err := startDatabase(t, stateDir, src); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("startup on the old target after activation: %v", err)
	}
	requireRuntimeSuccess(t, startDatabase(t, stateDir, target)) // Stale files are not a dependency after cutover.
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "activate"))
	if got := readRuntimeInstallation(t, target); !got.Served || got.ServingOwner != original.Owner {
		t.Fatal("destination not claimed by the activating owner")
	}
}

func TestActivationRejectsWrongSourceAndPreviouslyServedTarget(t *testing.T) {
	stateDir, source := sqliteRuntime(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, source))
	binding := readRuntimeBinding(t, stateDir)
	backup := filepath.Join(t.TempDir(), "source-transfer")
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "export", "--output", backup))

	otherSource := openRuntimeStore(t, sqliteAt(filepath.Join(t.TempDir(), "other-source")), true)
	var wrongBackup bytes.Buffer
	requireRuntimeSuccess(t, otherSource.Export(t.Context(), &wrongBackup))
	wrongTarget := sqliteAt(filepath.Join(t.TempDir(), "other-target"))
	wrongStore := openRuntimeStore(t, wrongTarget, true)
	requireRuntimeSuccess(t, wrongStore.Import(t.Context(), &wrongBackup))
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", wrongTarget.SQLitePath)
	if err := runtimeMaintenance(t, "db", "activate"); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("activated a transfer from a different source: %v", err)
	}
	assertRuntimeBinding(t, stateDir, binding)

	target := sqliteAt(filepath.Join(t.TempDir(), "destination"))
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", target.SQLitePath)
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import", "--input", backup))
	// A verified typed backup can be used for recovery on genuinely fresh local
	// state; it does not need (and cannot use) activation without a binding.
	fresh := filepath.Join(t.TempDir(), "state")
	requireRuntimeSuccess(t, startDatabase(t, fresh, target))
	freshBinding := readRuntimeBinding(t, fresh)
	if freshBinding.ID == binding.ID || freshBinding.Owner == binding.Owner || freshBinding.TransferSourceID != "" {
		t.Fatal("fresh recovery failed to establish its own binding")
	}
	if err := runtimeMaintenance(t, "db", "activate"); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("rebound a destination that already serves a different mount: %v", err)
	}
	assertRuntimeBinding(t, stateDir, binding)
	assertRuntimeBinding(t, fresh, freshBinding)
}

func TestOfflineTOTPResetRequiresBoundUnlockedDatabase(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	reset := []string{"admin", "totp", "reset", "--confirm"}
	if err := runtimeMaintenance(t, reset...); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("unbound reset: %v", err)
	}
	if _, err := os.Stat(cfg.SQLitePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unbound reset created a database: %v", err)
	}
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	binding := readRuntimeBinding(t, stateDir)
	seedRuntimeAuth(t, cfg)

	if err := runtimeMaintenance(t, reset[:3]...); err == nil {
		t.Fatal("reset did not require confirmation")
	}
	locked, err := lockStateDir(stateDir)
	requireRuntimeSuccess(t, err)
	err = runtimeMaintenance(t, reset...)
	locked.close()
	if !errors.Is(err, errStateLocked) {
		t.Fatalf("reset while runtime owns the state lock: %v", err)
	}
	assertRuntimeAuth(t, cfg, true)

	foreign := sqliteAt(filepath.Join(t.TempDir(), "foreign"))
	requireRuntimeSuccess(t, startDatabase(t, filepath.Join(t.TempDir(), "state"), foreign))
	seedRuntimeAuth(t, foreign)
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", foreign.SQLitePath)
	if err := runtimeMaintenance(t, reset...); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("reset on another installation: %v", err)
	}
	assertRuntimeAuth(t, foreign, true)
	assertRuntimeAuth(t, cfg, true)

	t.Setenv("ROAMINAL_DB_SQLITE_PATH", cfg.SQLitePath)
	var out bytes.Buffer
	requireRuntimeSuccess(t, runMaintenance(t.Context(), reset, &out))
	if !strings.Contains(out.String(), "TOTP enrollment and existing login sessions reset") {
		t.Fatalf("unexpected reset output: %q", out.String())
	}
	assertRuntimeAuth(t, cfg, false)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	assertRuntimeBinding(t, stateDir, binding)
}

func TestOfflineFileImportDoesNotCreateOrClaimABoundStateTarget(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	original := readRuntimeBinding(t, stateDir)
	legacyCopy := t.TempDir()
	putRuntimeLegacyFiles(t, legacyCopy)
	cmd := []string{"db", "import-files", "--source", legacyCopy, "--apply"}

	other := sqliteAt(filepath.Join(t.TempDir(), "other"))
	t.Setenv("ROAMINAL_DB_SQLITE_PATH", other.SQLitePath)
	if err := runtimeMaintenance(t, cmd...); err == nil {
		t.Fatal("file import accepted a missing database despite the existing binding")
	}
	if _, err := os.Lstat(filepath.Dir(other.SQLitePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file import created a destination directory: %v", err)
	}
	store := openRuntimeStore(t, other, true)
	requireRuntimeSuccess(t, store.Close())
	before := readRuntimeFile(t, other.SQLitePath)
	if err := runtimeMaintenance(t, cmd...); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("file import against the wrong installation: %v", err)
	}
	if !bytes.Equal(before, readRuntimeFile(t, other.SQLitePath)) {
		t.Fatal("file import mutated the wrong installation")
	}
	assertRuntimeBinding(t, stateDir, original)
}

func runtimeMaintenance(t *testing.T, args ...string) error {
	t.Helper()
	return runMaintenance(t.Context(), args, io.Discard)
}

func seedRuntimeAuth(t *testing.T, cfg db.Config) {
	t.Helper()
	ctx := t.Context()
	store := openRuntimeStore(t, cfg, false)
	defer store.Close()
	enrolled := domain.TOTPEnrollmentRecord{FormatVersion: domain.TOTPFormatVersion,
		EnrollmentID: "11111111-1111-4111-8111-111111111111", Secret: "JBSWY3DPEHPK3PXP", LastAcceptedTimeStep: 7}
	requireRuntimeSuccess(t, store.SaveEnrollment(ctx, enrolled))
	now := time.Now().UTC().Truncate(time.Second)
	requireRuntimeSuccess(t, store.SaveAuth(ctx, []domain.AuthSessionRecord{{
		ID: "22222222-2222-4222-8222-222222222222", EnrollmentID: enrolled.EnrollmentID,
		PasswordFingerprint: strings.Repeat("a", 64), RefreshTokenHash: strings.Repeat("b", 64),
		CreatedAt: now, LastSeenAt: now, RotatedAt: now, RefreshExpiresAt: now.Add(time.Hour),
	}}))
}

func assertRuntimeAuth(t *testing.T, cfg db.Config, wantEnrolled bool) {
	t.Helper()
	store := openRuntimeStore(t, cfg, false)
	defer store.Close()
	enrolled, exists, err := store.LoadEnrollment(t.Context())
	requireRuntimeSuccess(t, err)
	loginSessions, err := store.LoadAuth(t.Context())
	requireRuntimeSuccess(t, err)
	if exists != wantEnrolled || (wantEnrolled && (enrolled.LastAcceptedTimeStep != 7 || len(loginSessions) != 1)) || (!wantEnrolled && len(loginSessions) != 0) {
		t.Fatalf("unexpected auth state: enrolled=%t, login sessions=%d", exists, len(loginSessions))
	}
}

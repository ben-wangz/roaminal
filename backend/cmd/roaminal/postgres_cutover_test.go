package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

// The PostgreSQL test script provides a dedicated empty schema for the CLI.
// This rehearsal does not start HTTP, use a real PVC, or change a deployment.
func TestPostgresCLICutoverFromLegacyFiles(t *testing.T) {
	dsn := os.Getenv("ROAMINAL_TEST_POSTGRES_CLI_DSN")
	if dsn == "" || os.Getenv("ROAMINAL_TEST_POSTGRES_DISPOSABLE") != "1" {
		t.Skip("requires the disposable TLS PostgreSQL CLI fixture")
	}
	stateDir, source := sqliteRuntime(t)
	legacyCopy := filepath.Join(t.TempDir(), "state")
	instanceID := domain.ConnectionInstanceID("10000000-0000-4000-8000-000000000000")
	payload := []byte("offline cutover fixture\r\n")
	legacyState := seedLegacyCutover(t, stateDir, instanceID, payload)
	seedLegacyCutover(t, legacyCopy, instanceID, payload)
	for _, mode := range []string{"--dry-run", "--apply", "--verify"} {
		requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import-files", "--source", legacyCopy, mode))
	}
	for _, path := range []string{filepath.Join(legacyState.Root, "2fa-secret"), legacyState.ConnectionInstancePath(instanceID.String()),
		legacyState.ConnectionSnapshotPath(instanceID.String()), legacyState.AuditConnectionInstancePath(instanceID.String()),
		legacyState.AuditConnectionSnapshotPath(instanceID.String())} {
		requireRuntimeSuccess(t, os.Chmod(path, 0o660))
	}
	if err := startDatabase(t, stateDir, source); err == nil {
		t.Fatal("first startup accepted group-accessible legacy records")
	}
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "prepare-state", "--state-dir", stateDir, "--create-sqlite-dir", "--repair-legacy-files"))
	requireRuntimeSuccess(t, startDatabase(t, stateDir, source))
	original := readRuntimeBinding(t, stateDir)
	backup := filepath.Join(t.TempDir(), "backup")
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "export", "--output", backup))

	t.Setenv("ROAMINAL_DB_DRIVER", db.Postgres)
	t.Setenv("ROAMINAL_DB_POSTGRES_DSN", dsn)
	target, err := db.FromEnv(stateDir)
	requireRuntimeSuccess(t, err)
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import", "--input", backup))
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import", "--input", backup))
	if err := startDatabase(t, stateDir, target); !errors.Is(err, errDatabaseBinding) {
		t.Fatalf("driver switch without activation: %v", err)
	}
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "activate"))
	requireRuntimeSuccess(t, startDatabase(t, stateDir, target))
	activated := readRuntimeBinding(t, stateDir)
	if activated.Driver != db.Postgres || activated.TransferSourceID != original.ID || activated.Owner != original.Owner {
		t.Fatal("activation did not preserve state ownership and exact source lineage")
	}
	restored := openRuntimeStore(t, target, false)
	defer restored.Close()
	enrollment, enrolled, err := restored.LoadEnrollment(t.Context())
	requireRuntimeSuccess(t, err)
	if !enrolled || enrollment.LastAcceptedTimeStep != 7 {
		t.Fatal("migration did not preserve TOTP replay protection")
	}
	meta, err := restored.LoadConnectionInstance(t.Context(), instanceID)
	requireRuntimeSuccess(t, err)
	header, data, err := restored.LoadSnapshot(t.Context(), instanceID)
	requireRuntimeSuccess(t, err)
	if meta.AutomaticTitle != "Cutover fixture" || header.ThroughSequence != "8" || !bytes.Equal(data, payload) {
		t.Fatal("legacy connection instance or snapshot did not survive cutover")
	}
}

func seedLegacyCutover(t *testing.T, dir string, id domain.ConnectionInstanceID, payload []byte) *persistence.Store {
	t.Helper()
	store, err := persistence.New(dir)
	requireRuntimeSuccess(t, err)
	requireRuntimeSuccess(t, store.SaveTOTPEnrollment(domain.TOTPEnrollmentRecord{
		FormatVersion: domain.TOTPFormatVersion, EnrollmentID: "11111111-1111-4111-8111-111111111111",
		Secret: "JBSWY3DPEHPK3PXP", LastAcceptedTimeStep: 7,
	}))
	now := time.Date(2026, time.March, 1, 12, 30, 0, 123456789, time.UTC)
	requireRuntimeSuccess(t, store.SaveConnectionInstance(domain.ConnectionInstanceMeta{
		ID: id.String(), BackendRuntimeID: "legacy", ConnectionDefinitionID: "local", AutomaticTitle: "Cutover fixture",
		Type: "local", Purpose: "interactive", Lifecycle: "live", SourceState: "current", Cols: 80, Rows: 24,
		InitialCwd: "/workspace", Cwd: "/workspace", CreatedAt: now, UpdatedAt: now,
	}))
	requireRuntimeSuccess(t, store.SaveSnapshot(id.String(), domain.SnapshotHeader{
		Cols: 80, Rows: 24, ScrollbackLines: 1000, ThroughSequence: "8",
	}, payload))
	// The file-era writer could stop after archiving but before deleting active.
	requireRuntimeSuccess(t, store.ArchiveConnectionInstance(id.String()))
	return store
}

package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConnectionInstanceLayoutUsesPerInstanceFiles(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	now := time.Now().UTC()
	meta := ConnectionInstanceMeta{ID: id, BackendRuntimeID: "runtime", ConnectionDefinitionID: "local", Type: "local", Purpose: "interactive", Lifecycle: "live", SourceState: "current", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, TerminalType: "screen-256color", CreatedAt: now, UpdatedAt: now}
	if err := store.SaveConnectionInstance(meta); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(store.ConnectionInstancePath(id)) != "metadata.json" || filepath.Base(store.ConnectionSnapshotPath(id)) != "terminal.snapshot" {
		t.Fatalf("unexpected paths: %s %s", store.ConnectionInstancePath(id), store.ConnectionSnapshotPath(id))
	}
	loaded, err := store.LoadConnectionInstance(id)
	if err != nil || loaded.ConnectionDefinitionID != "local" || loaded.Type != "local" || loaded.TerminalType != "screen-256color" {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
}

func TestLegacySessionMigrationFailsWithActionableBackup(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "legacy.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root); err == nil || !strings.Contains(err.Error(), "migration blocked") {
		t.Fatalf("error=%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "migrations"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("migration backup missing: %v", err)
	}
}

func TestArchiveConnectionInstanceCopiesMaterialBeforeActiveDelete(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "22222222-2222-4222-8222-222222222222"
	now := time.Now().UTC()
	meta := ConnectionInstanceMeta{ID: id, BackendRuntimeID: "runtime", ConnectionDefinitionID: "local", Type: "local", Purpose: "interactive", Lifecycle: "exited", SourceState: "current", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}
	if err := store.SaveConnectionInstance(meta); err != nil {
		t.Fatal(err)
	}
	header := SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 100, ThroughSequence: "1"}
	if err := store.SaveSnapshot(id, header, []byte("exit\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := store.ArchiveConnectionInstance(id); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteConnectionInstance(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.ConnectionInstancePath(id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active metadata still exists: %v", err)
	}
	archived, err := os.ReadFile(store.AuditConnectionInstancePath(id))
	if err != nil || len(archived) == 0 {
		t.Fatalf("archived metadata unavailable: %v", err)
	}
	auditSnapshot, err := os.ReadFile(store.AuditConnectionSnapshotPath(id))
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := DecodeSnapshot(auditSnapshot)
	if err != nil || string(payload) != "exit\r\n" {
		t.Fatalf("archived snapshot=%q err=%v", payload, err)
	}
}

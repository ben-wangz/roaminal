package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConnectionLayoutUsesPerInstanceFiles(t *testing.T) {
	store, err := NewConnection(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	now := time.Now().UTC()
	meta := SessionMeta{ID: id, BackendRuntimeID: "runtime", ConnectionDefinitionID: "local", Type: "local", Purpose: "interactive", Lifecycle: "live", SourceState: "current", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}
	if err := store.SaveSession(meta); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(store.SessionPath(id)) != "metadata.json" || filepath.Base(store.SnapshotPath(id)) != "terminal.snapshot" {
		t.Fatalf("unexpected paths: %s %s", store.SessionPath(id), store.SnapshotPath(id))
	}
	loaded, err := store.LoadSession(id)
	if err != nil || loaded.ConnectionDefinitionID != "local" || loaded.Type != "local" {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
}

func TestConnectionLayoutRejectsLegacySessions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "legacy.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConnection(root); !errors.Is(err, ErrLegacySessions) {
		t.Fatalf("error=%v", err)
	}
}

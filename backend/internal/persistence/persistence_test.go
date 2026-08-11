package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotRoundTripAndCorruptionIsolation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("\x1b[2Jhello\n")
	if err := store.SaveSnapshot("11111111-1111-4111-8111-111111111111", SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 1000, ThroughSequence: "4"}, payload); err != nil {
		t.Fatal(err)
	}
	header, got, err := store.LoadSnapshot("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if header.ByteLength != len(payload) || string(got) != string(payload) {
		t.Fatalf("round trip mismatch: %+v %q", header, got)
	}
	if err := os.MkdirAll(filepath.Dir(store.SnapshotPath("11111111-1111-4111-8111-111111111111")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.SnapshotPath("11111111-1111-4111-8111-111111111111"), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadSnapshot("11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("expected corruption error")
	}
	entries, _ := os.ReadDir(filepath.Dir(store.SnapshotPath("11111111-1111-4111-8111-111111111111")))
	found := false
	for _, entry := range entries {
		if len(entry.Name()) > len(".snapshot.corrupt.") && entry.Name() != "11111111-1111-4111-8111-111111111111.snapshot" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected quarantined snapshot")
	}
}

func TestConnectionMetadataV1MigratesAndSavesAsV2(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	now := time.Now().UTC().Truncate(time.Second)
	legacy := fmt.Sprintf(`{"formatVersion":1,"connectionInstanceId":%q,"connectionDefinitionId":"local","type":"local","purpose":"interactive","lifecycle":"live","sourceState":"current","automaticTitle":"shell","initialCwd":"/workspace","cwd":"/workspace","cols":80,"rows":24,"createdAt":%q,"updatedAt":%q}`,
		id, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err := os.MkdirAll(filepath.Dir(store.SessionPath(id)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.SessionPath(id), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := store.LoadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FormatVersion != ConnectionFormatVersion || meta.AutomaticTitle != "shell" || meta.EffectiveTitle() != "shell" || meta.TitleOverride != nil {
		t.Fatalf("unexpected migrated metadata: %+v", meta)
	}
	if err := store.SaveSession(meta); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.SessionPath(id))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"formatVersion": 2`) || !strings.Contains(string(data), `"automaticTitle": "shell"`) || strings.Contains(string(data), `"title":`) || strings.Contains(string(data), `"executions"`) {
		t.Fatalf("metadata was not written as v2: %s", data)
	}
}

func TestAmbiguousStateLayoutFailsClosed(t *testing.T) {
	root := t.TempDir()
	id := "11111111-1111-4111-8111-111111111111"
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "state", "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", id+".json"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "sessions", id+".json"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root); !errors.Is(err, ErrAmbiguousStateLayout) {
		t.Fatalf("expected ambiguous layout error, got %v", err)
	}
}

func TestValidateTitleOverride(t *testing.T) {
	for _, value := range []string{"", "   ", "line\nfeed", "\u202ehidden"} {
		if err := ValidateTitleOverride(value); err == nil {
			t.Fatalf("expected invalid title %q", value)
		}
	}
	if err := ValidateTitleOverride("  useful title  "); err != nil {
		t.Fatal(err)
	}
}

func TestPersistenceDegradedTracksSessionsIndependently(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := "11111111-1111-4111-8111-111111111111"
	second := "22222222-2222-4222-8222-222222222222"
	store.MarkSessionDegraded(first)
	store.MarkSessionDegraded(second)
	if !store.PersistenceDegraded() {
		t.Fatal("expected degraded state")
	}
	now := time.Now().UTC()
	if err := store.SaveSession(SessionMeta{ID: first, InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if !store.PersistenceDegraded() {
		t.Fatal("a second failed session must keep persistence degraded")
	}
	store.clearSessionError(second)
	if store.PersistenceDegraded() {
		t.Fatal("expected healthy state after all session checkpoints recover")
	}
}

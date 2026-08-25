package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestWorkspaceLayoutSaveUsesAtomicRevisionCheck(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).Workspace
	id := domain.AuthenticationSessionID("auth-session")
	initial := domain.ConnectionInstanceLayout{Revision: 1, GroupOrder: []string{domain.UngroupedConnectionInstanceGroupID}}
	if err := repository.SaveWorkspaceLayout(nil, id, initial, 0); err != nil {
		t.Fatal(err)
	}
	left := initial
	left.Revision = 2
	left.UngroupedConnectionInstanceIDs = []string{"11111111-1111-4111-8111-111111111111"}
	right := initial
	right.Revision = 2
	right.UngroupedConnectionInstanceIDs = []string{"22222222-2222-4222-8222-222222222222"}
	var group sync.WaitGroup
	group.Add(2)
	results := make(chan error, 2)
	go func() { defer group.Done(); results <- repository.SaveWorkspaceLayout(nil, id, left, 1) }()
	go func() { defer group.Done(); results <- repository.SaveWorkspaceLayout(nil, id, right, 1) }()
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for saveErr := range results {
		switch {
		case saveErr == nil:
			successes++
		case saveErr == ports.ErrRevisionConflict:
			conflicts++
		default:
			t.Fatalf("unexpected save error: %v", saveErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("save results = successes %d conflicts %d", successes, conflicts)
	}
	loaded, found, err := repository.LoadWorkspaceLayout(nil, id)
	if err != nil || !found || loaded.Revision != 2 || len(loaded.UngroupedConnectionInstanceIDs) != 1 {
		t.Fatalf("loaded layout = %#v found=%v err=%v", loaded, found, err)
	}
}

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
	if err := os.MkdirAll(filepath.Dir(store.ConnectionSnapshotPath("11111111-1111-4111-8111-111111111111")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ConnectionSnapshotPath("11111111-1111-4111-8111-111111111111"), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadSnapshot("11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("expected corruption error")
	}
	entries, _ := os.ReadDir(filepath.Dir(store.ConnectionSnapshotPath("11111111-1111-4111-8111-111111111111")))
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

func TestConnectionMetadataV1MigratesAndSavesAsV3(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	now := time.Now().UTC().Truncate(time.Second)
	legacy := fmt.Sprintf(`{"formatVersion":1,"connectionInstanceId":%q,"connectionDefinitionId":"local","type":"local","purpose":"interactive","lifecycle":"live","sourceState":"current","automaticTitle":"shell","initialCwd":"/workspace","cwd":"/workspace","cols":80,"rows":24,"createdAt":%q,"updatedAt":%q}`,
		id, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err := os.MkdirAll(filepath.Dir(store.ConnectionInstancePath(id)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ConnectionInstancePath(id), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := store.LoadConnectionInstance(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FormatVersion != ConnectionFormatVersion || meta.AutomaticTitle != "shell" || meta.EffectiveTitle() != "shell" || meta.TitleOverride != nil {
		t.Fatalf("unexpected migrated metadata: %+v", meta)
	}
	if err := store.SaveConnectionInstance(meta); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.ConnectionInstancePath(id))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"formatVersion": 3`) || !strings.Contains(string(data), `"automaticTitle": "shell"`) || strings.Contains(string(data), `"title":`) || strings.Contains(string(data), `"executions"`) {
		t.Fatalf("metadata was not written as v3: %s", data)
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

func TestPersistenceDegradedTracksConnectionInstancesIndependently(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := "11111111-1111-4111-8111-111111111111"
	second := "22222222-2222-4222-8222-222222222222"
	store.MarkConnectionInstanceDegraded(first)
	store.MarkConnectionInstanceDegraded(second)
	if !store.PersistenceDegraded() {
		t.Fatal("expected degraded state")
	}
	now := time.Now().UTC()
	if err := store.SaveConnectionInstance(ConnectionInstanceMeta{ID: first, InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if !store.PersistenceDegraded() {
		t.Fatal("a second failed connection instance must keep persistence degraded")
	}
	store.clearConnectionInstanceError(second)
	if store.PersistenceDegraded() {
		t.Fatal("expected healthy state after all connection-instance checkpoints recover")
	}
}

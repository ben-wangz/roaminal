package db

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func workspaceTestLayout(revision uint64) domain.ConnectionInstanceLayout {
	return domain.ConnectionInstanceLayout{
		Revision: revision,
		GroupOrder: []string{"22222222-2222-4222-8222-222222222222", domain.UngroupedConnectionInstanceGroupID,
			"11111111-1111-4111-8111-111111111111"},
		Groups: []domain.ConnectionInstanceGroup{
			{GroupID: "11111111-1111-4111-8111-111111111111", Name: "First", ConnectionInstanceIDs: []string{}},
			{GroupID: "22222222-2222-4222-8222-222222222222", Name: "Second", ConnectionInstanceIDs: []string{
				"44444444-4444-4444-8444-444444444444", "33333333-3333-4333-8333-333333333333",
			}},
		},
		UngroupedConnectionInstanceIDs: []string{"66666666-6666-4666-8666-666666666666", "55555555-5555-4555-8555-555555555555"},
	}
}

func TestSQLiteWorkspaceUploadRepositories(t *testing.T) {
	s, _ := sqliteStore(t)
	testWorkspaceUploadRepositories(t, s)
}

func testWorkspaceUploadRepositories(t *testing.T, s *Store) {
	t.Helper()
	t.Run("workspace", func(t *testing.T) {
		ctx := context.Background()
		id := domain.AuthenticationSessionID("auth-session")
		if _, found, err := s.LoadWorkspaceLayout(ctx, id); found || err != nil {
			t.Fatalf("missing layout: found=%v err=%v", found, err)
		}
		if err := s.SaveWorkspaceLayout(ctx, id, workspaceTestLayout(2), 1); !errors.Is(err, ports.ErrRevisionConflict) {
			t.Fatalf("nonzero expected revision created a layout: %v", err)
		}
		if err := s.SaveWorkspaceLayout(ctx, id, workspaceTestLayout(2), 0); !errors.Is(err, ports.ErrRevisionConflict) {
			t.Fatalf("nonsequential revision accepted: %v", err)
		}
		invalid := workspaceTestLayout(1)
		invalid.GroupOrder = nil
		if err := s.SaveWorkspaceLayout(ctx, id, invalid, 0); err == nil || errors.Is(err, ports.ErrRevisionConflict) {
			t.Fatalf("invalid layout did not return a validation error: %v", err)
		}
		initial := workspaceTestLayout(1)
		initial.Groups[0].ConnectionInstanceIDs = nil
		if err := s.SaveWorkspaceLayout(nil, id, initial, 0); err != nil {
			t.Fatal(err)
		}
		if err := s.SaveWorkspaceLayout(ctx, id, workspaceTestLayout(3), 1); !errors.Is(err, ports.ErrRevisionConflict) {
			t.Fatalf("update skipped a revision: %v", err)
		}
		assertWorkspaceTestLayout(t, s, id, workspaceTestLayout(1))
	})
	t.Run("upload", func(t *testing.T) { testUploadRepository(t, s) })
}

func assertWorkspaceTestLayout(t *testing.T, s *Store, id domain.AuthenticationSessionID, want domain.ConnectionInstanceLayout) {
	t.Helper()
	got, found, err := s.LoadWorkspaceLayout(context.Background(), id)
	if !found || err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("load layout: got=%+v found=%v err=%v want=%+v", got, found, err, want)
	}
}

func TestSQLiteWorkspaceLayoutConcurrentCAS(t *testing.T) {
	s, config := sqliteStore(t)
	ctx := context.Background()
	other, err := Open(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	id := domain.AuthenticationSessionID("concurrent-auth")
	type result struct {
		layout domain.ConnectionInstanceLayout
		err    error
	}
	for expected := uint64(0); expected < 2; expected++ {
		start, results := make(chan struct{}), make(chan result, 2)
		for i, store := range []*Store{s, other} {
			layout := workspaceTestLayout(expected + 1)
			layout.Groups[0].Name = []string{"Left", "Right"}[i]
			go func() {
				<-start
				results <- result{layout, store.SaveWorkspaceLayout(ctx, id, layout, expected)}
			}()
		}
		close(start)
		var winner domain.ConnectionInstanceLayout
		conflicts := 0
		for range 2 {
			r := <-results
			switch {
			case r.err == nil:
				winner = r.layout
			case errors.Is(r.err, ports.ErrRevisionConflict):
				conflicts++
			default:
				t.Fatalf("CAS at revision %d: %v", expected, r.err)
			}
		}
		if conflicts != 1 {
			t.Fatalf("CAS at revision %d: got %d conflicts, want one", expected, conflicts)
		}
		assertWorkspaceTestLayout(t, other, id, winner)
	}
}

func TestSQLiteWorkspaceLayoutImport(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	for id, revision := range map[domain.AuthenticationSessionID]uint64{"legacy-zero": 0, "legacy-high": 1 << 63} {
		layout := workspaceTestLayout(revision)
		if err := s.ImportWorkspaceLayout(ctx, id, layout); err != nil {
			t.Fatal(err)
		}
		updated := workspaceTestLayout(revision + 1)
		if err := s.ImportWorkspaceLayout(ctx, id, updated); !errors.Is(err, ports.ErrRevisionConflict) {
			t.Fatalf("import replaced an existing layout: %v", err)
		}
		assertWorkspaceTestLayout(t, s, id, layout)
		if err := s.SaveWorkspaceLayout(ctx, id, updated, revision); err != nil {
			t.Fatalf("advance imported revision %d: %v", revision, err)
		}
		assertWorkspaceTestLayout(t, s, id, updated)
	}
}

func uploadTestRecord() domain.UploadJobRecord {
	now := time.Date(2026, 6, 12, 0, 0, 0, 123456789, time.UTC)
	return domain.UploadJobRecord{
		ID: "upload-01", ConnectionInstanceID: "instance-01", RootRevision: "root-01", TargetPath: ".",
		Status: "partial-failure", Transport: "sftp", ConflictPolicy: "update-if-newer", BytesSent: 2, BytesTotal: 4,
		CurrentPath: "docs/b.md", StagingPath: "/staging/upload-01", CreatedAt: now, UpdatedAt: now.Add(time.Second),
		Files: []domain.UploadFileRecord{
			{Part: "part-b", RelativePath: "docs/b.md", Size: 2, StagedPath: "/staging/b", ModifiedAt: now},
			{Part: "part-a", RelativePath: "a.md", Size: 2, StagedPath: "/staging/a"},
		},
		Failures: []domain.UploadFailureRecord{{Path: "docs/b.md", Code: "conflict", Error: "older file"}, {Path: "a.md", Code: "failed"}},
	}
}

func testUploadRepository(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	want := uploadTestRecord()
	if _, err := s.LoadUpload(ctx, want.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing upload: %v", err)
	}
	if _, err := s.LoadUpload(ctx, "../invalid"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("load invalid ID: %v", err)
	}
	if err := s.DeleteUpload(ctx, "../invalid"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("delete invalid ID: %v", err)
	}
	if err := s.SaveUpload(ctx, want); err != nil {
		t.Fatal(err)
	}
	if want.Files[0].StagedPath == "" {
		t.Fatal("save changed the caller's staged path")
	}
	assertUploadTestRecord(t, s, want)
	want.Status, want.BytesSent, want.CurrentPath = "completed", want.BytesTotal, ""
	want.UpdatedAt = want.UpdatedAt.Add(time.Second)
	want.Failures, want.Files = nil, []domain.UploadFileRecord{}
	if err := s.SaveUpload(ctx, want); err != nil {
		t.Fatal(err)
	}
	assertUploadTestRecord(t, s, want)
	for range 2 {
		if err := s.DeleteUpload(ctx, want.ID); err != nil {
			t.Fatalf("delete upload: %v", err)
		}
	}
	if _, err := s.LoadUpload(ctx, want.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted upload: %v", err)
	}
}

func assertUploadTestRecord(t *testing.T, s *Store, want domain.UploadJobRecord) {
	t.Helper()
	got, err := s.LoadUpload(context.Background(), want.ID)
	want.StagingPath, want.Files = "", slices.Clone(want.Files)
	for i := range want.Files {
		want.Files[i].StagedPath = ""
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("load upload: got=%+v err=%v want=%+v", got, err, want)
	}
}

func TestSQLiteWorkspaceUploadCanceledContext(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, loadLayoutErr := s.LoadWorkspaceLayout(ctx, "auth-session")
	_, loadUploadErr := s.LoadUpload(ctx, "../invalid")
	for operation, err := range map[string]error{
		"load layout":   loadLayoutErr,
		"save layout":   s.SaveWorkspaceLayout(ctx, "auth-session", domain.ConnectionInstanceLayout{}, 0),
		"import layout": s.ImportWorkspaceLayout(ctx, "auth-session", domain.ConnectionInstanceLayout{}),
		"load upload":   loadUploadErr,
		"save upload":   s.SaveUpload(ctx, domain.UploadJobRecord{}),
		"delete upload": s.DeleteUpload(ctx, "../invalid"),
	} {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("%s: got %v, want context.Canceled", operation, err)
		}
	}
}

func TestSQLiteUploadValidation(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		change func(*domain.UploadJobRecord)
	}{
		{"id", func(r *domain.UploadJobRecord) { r.ID = "../invalid" }},
		{"instance", func(r *domain.UploadJobRecord) { r.ConnectionInstanceID = "" }},
		{"root", func(r *domain.UploadJobRecord) { r.RootRevision = "" }},
		{"target", func(r *domain.UploadJobRecord) { r.TargetPath = "" }},
		{"status", func(r *domain.UploadJobRecord) { r.Status = "pending" }},
		{"policy", func(r *domain.UploadJobRecord) { r.ConflictPolicy = "unknown" }},
		{"bytes", func(r *domain.UploadJobRecord) { r.BytesSent = r.BytesTotal + 1 }},
		{"timestamps", func(r *domain.UploadJobRecord) { r.UpdatedAt = r.CreatedAt.Add(-time.Second) }},
		{"path", func(r *domain.UploadJobRecord) { r.Files[0].RelativePath = "docs/../b.md" }},
		{"file size", func(r *domain.UploadJobRecord) { r.Files[0].Size = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := uploadTestRecord()
			tc.change(&record)
			if err := s.SaveUpload(ctx, record); err == nil {
				t.Fatal("invalid upload accepted")
			}
		})
	}
}

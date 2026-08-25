package persistence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestUploadRepositoryRoundTripAndDelete(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	record := domain.UploadJobRecord{
		ID:                   "upload-01",
		ConnectionInstanceID: "11111111-1111-4111-8111-111111111111",
		RootRevision:         "root-revision",
		TargetPath:           ".",
		ConflictPolicy:       "refuse",
		Status:               "queued",
		Transport:            "pending",
		BytesTotal:           12,
		Files:                []domain.UploadFileRecord{{Part: "file-0", RelativePath: "docs/readme.md", Size: 12, StagedPath: "/state/uploads/upload-01/docs/readme.md"}},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := store.SaveUpload(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadUpload(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != record.ID || loaded.Files[0].RelativePath != "docs/readme.md" || loaded.BytesTotal != 12 {
		t.Fatalf("unexpected upload record: %+v", loaded)
	}
	if err := store.DeleteUpload(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadUpload(context.Background(), record.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted upload, got %v", err)
	}
}

func TestUploadRepositoryRejectsUnknownFieldsAndInvalidVersion(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := store.uploadPath("upload-02")
	if err := os.MkdirAll(store.uploadDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"formatVersion":3,"record":{},"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadUpload(context.Background(), "upload-02"); err == nil {
		t.Fatal("expected strict upload decode failure")
	}
}

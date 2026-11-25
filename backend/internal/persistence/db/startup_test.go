package db

import (
	"slices"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestSQLiteStartupRecoversUploads(t *testing.T) {
	s, cfg := sqliteStore(t)
	testStartupRecoversUploads(t, s, cfg)
}

func TestPostgresStartupRecoversUploads(t *testing.T) {
	s, cfg := postgresStore(t)
	testStartupRecoversUploads(t, s, cfg)
}

func testStartupRecoversUploads(t *testing.T, s *Store, cfg Config) {
	t.Helper()
	ctx := t.Context()
	owner := strings.Repeat("53", 16)
	if err := s.Ready(ctx); err == nil {
		t.Fatal("unused database was ready")
	}
	requireTransferTestSuccess(t, s.PrepareServing(ctx, owner, false, ""))
	requireTransferTestSuccess(t, s.Ready(ctx))
	statuses := []string{"queued", "running", "completed"}
	for _, status := range statuses {
		record := uploadTestRecord()
		record.ID, record.Status = "job-"+status, status
		requireTransferTestSuccess(t, s.SaveUpload(ctx, record))
	}
	requireTransferTestSuccess(t, s.Close())
	if err := s.Ready(ctx); err == nil {
		t.Fatal("closed database was ready")
	}
	reopened, err := OpenExisting(ctx, cfg)
	requireTransferTestSuccess(t, err)
	defer reopened.Close()
	for range 2 {
		requireTransferTestSuccess(t, reopened.PrepareServing(ctx, owner, false, ""))
		requireTransferTestSuccess(t, reopened.Ready(ctx))
		for _, status := range statuses {
			job, err := reopened.LoadUpload(ctx, "job-"+status)
			requireTransferTestSuccess(t, err)
			want := uploadTestRecord()
			if status != "completed" {
				status = "failed"
				want.Failures = append(want.Failures, domain.UploadFailureRecord{Code: "filesystem_upload_failed", Error: "Upload interrupted by restart"})
			}
			if job.Status != status || !slices.Equal(job.Failures, want.Failures) || job.BytesSent != want.BytesSent ||
				job.BytesTotal != want.BytesTotal || !job.UpdatedAt.Equal(want.UpdatedAt) || job.StagingPath != "" {
				t.Fatalf("upload %s was not recovered idempotently", job.ID)
			}
		}
	}
}

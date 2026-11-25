package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestPrepareUploadStagingRemovesOnlyOwnedStaging(t *testing.T) {
	state := t.TempDir()
	if err := PrepareUploadStaging(state); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(state, uploadStagingDirectory)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("staging directory not private: %v", err)
	}
	orphan := filepath.Join(root, "upload-interrupted", "subdirectory")
	legacy := filepath.Join(state, "uploads", "upload-legacy")
	unrelated := filepath.Join(root, "unrelated")
	for _, path := range []string{orphan, legacy, unrelated} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(orphan, "payload"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "upload-link")
	if err := os.Symlink(legacy, link); err != nil {
		t.Fatal(err)
	}
	if err := PrepareUploadStaging(state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(orphan)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted staging directory survived: %v", err)
	}
	for _, path := range []string{legacy, unrelated, link} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("cleanup changed %s: %v", path, err)
		}
	}
}

func TestPrepareUploadStagingRejectsSymlink(t *testing.T) {
	state, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(state, uploadStagingDirectory)); err != nil {
		t.Fatal(err)
	}
	if err := PrepareUploadStaging(state); err == nil {
		t.Fatal("symlink was accepted as a staging directory")
	}
}

func TestUploadByteBudgetSpansReceivingQueuedAndRunning(t *testing.T) {
	receiving, allowReceive := make(chan struct{}), make(chan struct{})
	queued, allowSave := make(chan struct{}), make(chan struct{})
	running := make(chan struct{})
	remote := uploadTestRemote{resolve: func(ctx context.Context) (string, error) {
		close(running)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-t.Context().Done():
			return "", t.Context().Err()
		}
	}}
	repo := uploadTestRepository{UploadRepository: newMemoryUploadRepository(), save: func(ctx context.Context, record domain.UploadJobRecord) error {
		if record.Status != "queued" {
			return nil
		}
		close(queued)
		select {
		case <-allowSave:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	s := newStagingTestService(t, remote, repo)
	s.staging.byteLimit, s.staging.jobLimit = 6, 2
	source := uploadSource(4, "data")
	read := source.files
	source.files = func(stage func(string, io.Reader) error) error {
		close(receiving)
		select {
		case <-allowReceive:
			return read(stage)
		case <-t.Context().Done():
			return t.Context().Err()
		}
	}
	type result struct {
		status UploadStatus
		err    error
	}
	created := make(chan result, 1)
	go func() {
		status, err := s.CreateUpload(t.Context(), testUploadInstance, source)
		created <- result{status, err}
	}()
	reject := func(phase string) {
		t.Helper()
		candidate := uploadSource(3, "abc")
		candidate.files = func(func(string, io.Reader) error) error {
			t.Error("over-capacity request attempted to stage a file")
			return nil
		}
		if _, err := s.CreateUpload(t.Context(), "other-instance", candidate); !errors.Is(err, ErrUploadStagingCapacity) {
			t.Fatalf("%s: competing upload error = %v", phase, err)
		}
	}
	awaitUploadValue(t, receiving)
	reject("receiving before writing")
	close(allowReceive)
	awaitUploadValue(t, queued)
	reject("persisting queued upload")
	close(allowSave)
	first := awaitUploadValue(t, created)
	if first.err != nil || first.status.Status != "queued" {
		t.Fatalf("create returned %+v", first)
	}
	awaitUploadValue(t, running)
	reject("running")
	if _, err := s.CancelUpload(testUploadInstance, first.status.UploadID); err != nil {
		t.Fatal(err)
	}
	waitForStagingRelease(t, s)
	if status, err := s.UploadStatus(testUploadInstance, first.status.UploadID); err != nil || status.Status != "cancelled" {
		t.Fatalf("cancelled status = %+v, %v", status, err)
	}
}

func TestUploadJobBudgetCountsReceivingAndReleasesOnRequestCancel(t *testing.T) {
	s := newStagingTestService(t, uploadTestRemote{}, nil)
	s.staging.jobLimit = 1
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	readingManifest, staged := make(chan struct{}), make(chan struct{})
	allowManifest := make(chan struct{})
	source := uploadSource(4, "data")
	source.manifestReader = func() (UploadManifest, error) {
		close(readingManifest)
		select {
		case <-allowManifest:
			return source.manifest, nil
		case <-ctx.Done():
			return UploadManifest{}, ctx.Err()
		}
	}
	read := source.files
	source.files = func(stage func(string, io.Reader) error) error {
		if err := read(stage); err != nil {
			return err
		}
		close(staged)
		<-ctx.Done()
		return ctx.Err()
	}
	finished := make(chan error, 1)
	go func() {
		_, err := s.CreateUpload(ctx, testUploadInstance, source)
		finished <- err
	}()
	awaitUploadValue(t, readingManifest)
	candidate := stagedUploadSource{manifestReader: func() (UploadManifest, error) {
		t.Error("second request read its metadata with no job slots available")
		return UploadManifest{}, ErrUploadManifestInvalid
	}}
	if _, err := s.CreateUpload(t.Context(), testUploadInstance, candidate); !errors.Is(err, ErrUploadStagingCapacity) {
		t.Fatalf("second upload error = %v", err)
	}
	close(allowManifest)
	awaitUploadValue(t, staged)
	cancel()
	if err := awaitUploadValue(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation returned %v", err)
	}
	waitForStagingRelease(t, s)
	candidate.manifestReader = nil
	if _, err := s.CreateUpload(t.Context(), testUploadInstance, candidate); !errors.Is(err, ErrContentTooLarge) {
		t.Fatalf("next request not admitted after release: %v", err)
	}
}

func TestUploadCompletionReleasesStaging(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "scp"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	s := newStagingTestService(t, uploadTestRemote{resolve: func(context.Context) (string, error) { return "/workspace", nil }}, nil)
	queued, err := s.CreateUpload(t.Context(), testUploadInstance, uploadSource(4, "data"))
	if err != nil {
		t.Fatal(err)
	}
	waitForStagingRelease(t, s)
	if status, err := s.UploadStatus(testUploadInstance, queued.UploadID); err != nil || status.Status != "completed" || status.BytesSent != 4 {
		t.Fatalf("completed status = %+v, %v", status, err)
	}
}

func TestUploadStagingCleanupFailureKeepsReservation(t *testing.T) {
	budget := uploadStagingBudget{byteLimit: 4, jobLimit: 2}
	reservation, err := budget.admit()
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.reserveBytes(4); err != nil {
		t.Fatal(err)
	}
	reservation.path = "." // RemoveAll refuses this on any permission level.
	reservation.cleanup()
	if budget.jobs != 1 || budget.bytes != 4 {
		t.Fatalf("failed cleanup released budget: %d jobs, %d bytes", budget.jobs, budget.bytes)
	}
}

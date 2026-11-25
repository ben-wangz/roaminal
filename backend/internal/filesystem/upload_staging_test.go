package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

const testUploadInstance = "instance"

type stagedUploadSource struct {
	manifest       UploadManifest
	manifestReader func() (UploadManifest, error)
	files          func(stage func(string, io.Reader) error) error
}

func (s stagedUploadSource) ReadManifest() (UploadManifest, error) {
	if s.manifestReader != nil {
		return s.manifestReader()
	}
	return s.manifest, nil
}

func (s stagedUploadSource) ReadFiles(_ int64, stage func(string, io.Reader) error) error {
	if s.files != nil {
		return s.files(stage)
	}
	return nil
}

func uploadSource(size int64, payload string) stagedUploadSource {
	return stagedUploadSource{
		manifest: UploadManifest{ConflictPolicy: "overwrite", Files: []UploadManifestFile{{Part: "file-0", RelativePath: "fixture.txt", Size: size}}},
		files:    func(stage func(string, io.Reader) error) error { return stage("file-0", strings.NewReader(payload)) },
	}
}

type uploadTestRemote struct {
	ports.RemoteFileSystem
	currentRoot func() string
	resolve     func(context.Context) (string, error)
}

func (r uploadTestRemote) ConnectionInstance(id string) (ports.ConnectionInstanceView, error) {
	alias := "fixture"
	return ports.ConnectionInstanceView{ID: id, Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}, nil
}

func (r uploadTestRemote) ProbeConfiguredRoot(context.Context, string, string) (ports.RemoteRoot, error) {
	path := "/workspace"
	if r.currentRoot != nil {
		path = r.currentRoot()
	}
	return ports.RemoteRoot{AbsolutePath: path, Source: "configured", Status: "fallback"}, nil
}

func (r uploadTestRemote) ResolveUploadTarget(ctx context.Context, _, _, _ string) (string, error) {
	return r.resolve(ctx)
}

func (r uploadTestRemote) AcquireRemoteTransfer(context.Context, string) (ports.RemoteTransferLease, error) {
	return fakeTransferLease{}, nil
}

func (r uploadTestRemote) RsyncAvailable(context.Context, string) (bool, error) { return false, nil }

type uploadTestRepository struct {
	ports.UploadRepository
	save func(context.Context, domain.UploadJobRecord) error
}

func (r uploadTestRepository) SaveUpload(ctx context.Context, record domain.UploadJobRecord) error {
	if r.save != nil {
		if err := r.save(ctx, record); err != nil {
			return err
		}
	}
	return r.UploadRepository.SaveUpload(ctx, record)
}

func newStagingTestService(t *testing.T, remote uploadTestRemote, repo ports.UploadRepository) *Service {
	t.Helper()
	state := t.TempDir()
	if err := PrepareUploadStaging(state); err != nil {
		t.Fatal(err)
	}
	return NewWithRemote(remote, nil, repo, state)
}

func awaitUploadValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for upload")
		panic("unreachable")
	}
}

func waitForStagingRelease(t *testing.T, s *Service) {
	t.Helper()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		s.staging.mu.Lock()
		jobs, bytes := s.staging.jobs, s.staging.bytes
		s.staging.mu.Unlock()
		if jobs == 0 && bytes == 0 {
			entries, err := os.ReadDir(s.stagingRoot)
			if err != nil || len(entries) != 0 {
				t.Fatalf("staging not empty: %v, %v", entries, err)
			}
			return
		}
		select {
		case <-tick.C:
		case <-timeout.C:
			t.Fatalf("staging budget not released: %d jobs with %d bytes", jobs, bytes)
		}
	}
}

func TestCreateUploadRejectsAndReleasesStaging(t *testing.T) {
	saveErr := errors.New("save failed")
	for _, test := range []struct {
		name   string
		change func(*stagedUploadSource)
		want   error
	}{
		{"missing part", func(s *stagedUploadSource) { s.files = nil }, ErrContentUnavailable},
		{"unsafe target", func(s *stagedUploadSource) { s.manifest.TargetPath = "../outside" }, ErrPathOutsideRoot},
		{"unsafe file", func(s *stagedUploadSource) { s.manifest.Files[0].RelativePath = "../outside" }, ErrPathOutsideRoot},
		{"too many files", func(s *stagedUploadSource) { s.manifest.Files = make([]UploadManifestFile, MaxUploadFiles+1) }, ErrContentTooLarge},
		{"file too large", func(s *stagedUploadSource) { s.manifest.Files[0].Size = MaxUploadBytes + 1 }, ErrContentTooLarge},
		{"save failure", func(*stagedUploadSource) {}, saveErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := uploadTestRepository{UploadRepository: newMemoryUploadRepository(), save: func(context.Context, domain.UploadJobRecord) error { return saveErr }}
			s := newStagingTestService(t, uploadTestRemote{}, repo)
			source := uploadSource(4, "data")
			test.change(&source)
			status, err := s.CreateUpload(t.Context(), testUploadInstance, source)
			if !errors.Is(err, test.want) || status.UploadID != "" {
				t.Fatalf("create returned %+v, %v; want %v", status, err, test.want)
			}
			waitForStagingRelease(t, s)
		})
	}
}

func TestCreateUploadRevalidatesRootAfterReceivingFiles(t *testing.T) {
	for _, kind := range []string{"explicit revision", "implicit revision"} {
		t.Run(kind, func(t *testing.T) {
			path := "/old"
			s := newStagingTestService(t, uploadTestRemote{currentRoot: func() string { return path }}, nil)
			source := uploadSource(4, "data")
			if kind == "explicit revision" {
				root, err := s.Root(t.Context(), testUploadInstance)
				if err != nil {
					t.Fatal(err)
				}
				source.manifest.RootRevision = root.Revision
			}
			files := source.files
			source.files = func(stage func(string, io.Reader) error) error {
				err := files(stage)
				path = "/new"
				return err
			}
			status, err := s.CreateUpload(t.Context(), testUploadInstance, source)
			var changed *RootChangedError
			if !errors.As(err, &changed) || changed.Root.AbsolutePath != "/new" || status.UploadID != "" {
				t.Fatalf("expected root change, got %+v, %v", status, err)
			}
			waitForStagingRelease(t, s)
		})
	}
}

func TestStageFileDoesNotWritePastDeclaredSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	item := UploadManifestFile{Size: 1}
	err := stageUploadFile(t.Context(), path, item, strings.NewReader("extra data"), make([]byte, 64))
	if !errors.Is(err, ErrContentUnavailable) {
		t.Fatalf("size mismatch returned %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "e" {
		t.Fatalf("staged file exceeded the reservation: %q, %v", data, err)
	}
}

package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var (
	ErrUploadManifestInvalid = errors.New("filesystem upload manifest invalid")
	ErrUploadStagingCapacity = errors.New("local upload staging capacity exhausted")
)

// UploadSource reads a manifest followed by a stream of file parts. ReadFiles
// calls stage synchronously for each part; it must validate the end of the input
// and stop immediately if stage returns an error.
type UploadSource interface {
	ReadManifest() (UploadManifest, error)
	ReadFiles(total int64, stage func(name string, content io.Reader) error) error
}

func (s *Service) CreateUpload(ctx context.Context, id string, source UploadSource) (UploadStatus, error) {
	reservation, err := s.staging.admit()
	if err != nil {
		return UploadStatus{}, err
	}
	handedOff := false
	defer func() {
		if !handedOff {
			reservation.cleanup()
		}
	}()
	manifest, err := source.ReadManifest()
	if err != nil {
		return UploadStatus{}, err
	}
	root, err := s.rootForRevision(ctx, id, manifest.RootRevision)
	if err != nil {
		return UploadStatus{}, err
	}
	target, err := ValidateRelativePath(manifest.TargetPath)
	if err != nil {
		return UploadStatus{}, err
	}
	if manifest.ConflictPolicy == "" {
		manifest.ConflictPolicy = "refuse"
	}
	if manifest.ConflictPolicy != "refuse" && manifest.ConflictPolicy != "overwrite" && manifest.ConflictPolicy != "update-if-newer" {
		return UploadStatus{}, ErrInvalidPath
	}
	files, byPart, total, err := planUpload(manifest.Files)
	if err != nil {
		return UploadStatus{}, err
	}
	if err := reservation.reserveBytes(total); err != nil {
		return UploadStatus{}, err
	}
	if err := ctx.Err(); err != nil {
		return UploadStatus{}, err
	}
	if s.stagingRoot == "" {
		return UploadStatus{}, ErrUploadFailed
	}
	staging, err := os.MkdirTemp(s.stagingRoot, "upload-")
	if err != nil {
		return UploadStatus{}, stagingDiskError(err)
	}
	reservation.path = staging
	if err := receiveUploadFiles(ctx, source, staging, files, byPart, total); err != nil {
		return UploadStatus{}, err
	}
	// A slow request can outlive the root checked at admission. Resolve again
	// before accepting the job, as the transfer uses a snapshot of the root.
	if err := ctx.Err(); err != nil {
		return UploadStatus{}, err
	}
	root, err = s.rootForRevision(ctx, id, root.Revision)
	if err != nil {
		return UploadStatus{}, err
	}
	job, jobContext, err := s.queueUpload(ctx, id, manifest.ConflictPolicy, target, total, root, staging, files)
	if err != nil {
		return UploadStatus{}, err
	}
	handedOff = true
	status := job.snapshot()
	go s.runUpload(jobContext, id, manifest.ConflictPolicy, job, reservation)
	return status, nil
}

func planUpload(items []UploadManifestFile) ([]stagedUploadFile, map[string]int, int64, error) {
	if len(items) == 0 || len(items) > MaxUploadFiles {
		return nil, nil, 0, ErrContentTooLarge
	}
	files := make([]stagedUploadFile, len(items))
	byPart := make(map[string]int, len(items))
	seenPaths := make(map[string]bool, len(items))
	var total int64
	for i, item := range items {
		if _, duplicate := byPart[item.Part]; duplicate || item.Part == "" || item.Size < 0 {
			return nil, nil, 0, ErrInvalidPath
		}
		// Staging uses local paths. Bound their length before MkdirAll recurses.
		if len(item.RelativePath) >= 4096 {
			return nil, nil, 0, ErrInvalidPath
		}
		relative, err := ValidateRelativePath(item.RelativePath)
		if err != nil || relative == "." || seenPaths[relative] {
			return nil, nil, 0, pathErrOrInvalid(err)
		}
		if item.Size > MaxUploadBytes-total {
			return nil, nil, 0, ErrContentTooLarge
		}
		total += item.Size
		item.RelativePath = relative
		files[i].Manifest = item
		byPart[item.Part] = i
		seenPaths[relative] = true
	}
	return files, byPart, total, nil
}

func receiveUploadFiles(ctx context.Context, source UploadSource, staging string, files []stagedUploadFile, byPart map[string]int, total int64) error {
	buffer := make([]byte, 32<<10)
	err := source.ReadFiles(total, func(name string, content io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		i, ok := byPart[name]
		if !ok || files[i].Path != "" {
			return ErrUploadManifestInvalid
		}
		item := files[i].Manifest
		destination := filepath.Join(staging, filepath.FromSlash(item.RelativePath))
		if err := stageUploadFile(ctx, destination, item, content, buffer); err != nil {
			return err
		}
		files[i].Path = destination
		return nil
	})
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Path == "" {
			return ErrContentUnavailable
		}
	}
	return nil
}

func stageUploadFile(ctx context.Context, destination string, item UploadManifestFile, content io.Reader, buffer []byte) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return stagingDiskError(err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return stagingDiskError(err)
	}
	input := uploadContextReader{ctx, content}
	// Never write beyond the bytes already reserved for this file, not even
	// the extra byte used to detect a size mismatch.
	written, copyErr := io.CopyBuffer(struct{ io.Writer }{output}, io.LimitReader(input, item.Size), buffer)
	closeErr := output.Close()
	if copyErr != nil {
		return stagingDiskError(copyErr)
	}
	if closeErr != nil {
		return stagingDiskError(closeErr)
	}
	if written != item.Size {
		return ErrContentUnavailable
	}
	var extra [1]byte
	if _, err := io.ReadFull(input, extra[:]); err != io.EOF {
		if err != nil {
			return err
		}
		return ErrContentUnavailable
	}
	if !item.ModifiedAt.IsZero() {
		_ = os.Chtimes(destination, item.ModifiedAt, item.ModifiedAt)
	}
	return nil
}

type uploadContextReader struct {
	ctx context.Context
	io.Reader
}

func (r uploadContextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(buffer)
}

func stagingDiskError(err error) error {
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) {
		return ErrUploadStagingCapacity
	}
	return err
}

func (s *Service) queueUpload(ctx context.Context, id, policy, target string, total int64, root RootContext, staging string, files []stagedUploadFile) (*uploadJob, context.Context, error) {
	uploadID := s.newUploadID()
	jobContext, cancel := context.WithCancel(context.Background())
	job := &uploadJob{
		instanceID:     id,
		status:         UploadStatus{UploadID: uploadID, Status: "queued", Transport: "pending", TargetPath: target, BytesTotal: total, Failures: []UploadFailure{}},
		staging:        staging,
		files:          files,
		root:           root,
		conflictPolicy: policy,
		createdAt:      s.now().UTC(),
		clock:          s.clock,
		cancel:         cancel,
	}
	job.persist = func() { _ = s.persistUpload(context.Background(), job) }
	if err := s.persistUpload(ctx, job); err != nil {
		cancel()
		return nil, nil, err
	}
	s.mu.Lock()
	s.uploads[uploadID] = job
	s.mu.Unlock()
	return job, jobContext, nil
}

package filesystem

import (
	"context"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

func (s *Service) CreateUpload(ctx context.Context, id string, manifest UploadManifest, parts map[string]*multipart.FileHeader) (UploadStatus, error) {
	if s.remote == nil {
		return UploadStatus{}, ErrNoTransport
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
	if len(manifest.Files) == 0 || len(manifest.Files) > maxUploadFiles {
		return UploadStatus{}, ErrContentTooLarge
	}
	if s.stagingRoot != "" {
		if err := os.MkdirAll(s.stagingRoot, 0o700); err != nil {
			return UploadStatus{}, err
		}
	}
	staging, err := os.MkdirTemp(s.stagingRoot, "upload-")
	if err != nil {
		return UploadStatus{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	files := make([]stagedUploadFile, 0, len(manifest.Files))
	seenParts := make(map[string]bool, len(manifest.Files))
	seenPaths := make(map[string]bool, len(manifest.Files))
	var total int64
	for _, item := range manifest.Files {
		if item.Part == "" || seenParts[item.Part] || item.Size < 0 {
			return UploadStatus{}, ErrInvalidPath
		}
		seenParts[item.Part] = true
		relative, pathErr := ValidateRelativePath(item.RelativePath)
		if pathErr != nil || relative == "." || seenPaths[relative] {
			return UploadStatus{}, pathErrOrInvalid(pathErr)
		}
		item.RelativePath = relative
		seenPaths[relative] = true
		part := parts[item.Part]
		if part == nil || part.Size != item.Size {
			return UploadStatus{}, ErrContentUnavailable
		}
		if item.Size > maxUploadBytes-total {
			return UploadStatus{}, ErrContentTooLarge
		}
		total += item.Size
		destination := filepath.Join(staging, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return UploadStatus{}, err
		}
		input, openErr := part.Open()
		if openErr != nil {
			return UploadStatus{}, openErr
		}
		output, createErr := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if createErr != nil {
			_ = input.Close()
			return UploadStatus{}, createErr
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, item.Size+1))
		_ = input.Close()
		_ = output.Close()
		if copyErr != nil {
			return UploadStatus{}, copyErr
		}
		if written != item.Size {
			return UploadStatus{}, ErrContentUnavailable
		}
		if !item.ModifiedAt.IsZero() {
			_ = os.Chtimes(destination, item.ModifiedAt, item.ModifiedAt)
		}
		files = append(files, stagedUploadFile{Manifest: item, Path: destination})
	}
	uploadID := s.newUploadID()
	jobContext, cancel := context.WithCancel(context.Background())
	job := &uploadJob{
		instanceID:     id,
		status:         UploadStatus{UploadID: uploadID, Status: "queued", Transport: "pending", TargetPath: target, BytesTotal: total, Failures: []UploadFailure{}},
		staging:        staging,
		files:          files,
		root:           root,
		conflictPolicy: manifest.ConflictPolicy,
		createdAt:      s.now().UTC(),
		clock:          s.clock,
		cancel:         cancel,
	}
	job.persist = func() { _ = s.persistUpload(job) }
	if err := s.persistUpload(job); err != nil {
		cancel()
		_ = os.RemoveAll(staging)
		return UploadStatus{}, err
	}
	s.mu.Lock()
	s.uploads[uploadID] = job
	s.mu.Unlock()
	cleanup = false
	go s.runUpload(jobContext, id, manifest.ConflictPolicy, job)
	return job.snapshot(), nil
}

package filesystem

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

const (
	maxUploadBytes = int64(10 << 30)
	maxUploadFiles = 10_000
)

type uploadJob struct {
	mu             sync.Mutex
	instanceID     string
	status         UploadStatus
	staging        string
	files          []stagedUploadFile
	root           RootContext
	conflictPolicy string
	createdAt      time.Time
	clock          ports.Clock
	cancel         context.CancelFunc
	persist        func()
}

type stagedUploadFile struct {
	Manifest UploadManifestFile
	Path     string
}

func (s *Service) UploadStatus(instanceID, uploadID string) (UploadStatus, error) {
	s.mu.Lock()
	job := s.uploads[uploadID]
	s.mu.Unlock()
	if job != nil {
		if job.instanceID != instanceID {
			return UploadStatus{}, ErrUploadNotFound
		}
		return job.snapshot(), nil
	}
	record, err := s.uploadRepo.LoadUpload(context.Background(), uploadID)
	if err != nil {
		if !isUploadNotFound(err) {
			return UploadStatus{}, err
		}
		return UploadStatus{}, ErrUploadNotFound
	}
	if record.ConnectionInstanceID != instanceID {
		return UploadStatus{}, ErrUploadNotFound
	}
	return uploadStatusFromRecord(record), nil
}

func (s *Service) CancelUpload(instanceID, uploadID string) (UploadStatus, error) {
	s.mu.Lock()
	job := s.uploads[uploadID]
	s.mu.Unlock()
	if job == nil {
		record, err := s.uploadRepo.LoadUpload(context.Background(), uploadID)
		if isUploadNotFound(err) || err != nil {
			return UploadStatus{}, ErrUploadNotFound
		}
		if record.ConnectionInstanceID != instanceID {
			return UploadStatus{}, ErrUploadNotFound
		}
		status := uploadStatusFromRecord(record)
		if status.Status != "completed" && status.Status != "failed" && status.Status != "partial-failure" && status.Status != "cancelled" {
			status.Status = "cancelled"
			record.Status = status.Status
			record.CurrentPath = ""
			record.UpdatedAt = s.now().UTC()
			_ = s.uploadRepo.SaveUpload(context.Background(), record)
		}
		return status, nil
	}
	if job.instanceID != instanceID {
		return UploadStatus{}, ErrUploadNotFound
	}
	job.mu.Lock()
	terminal := job.status.Status == "completed" || job.status.Status == "failed" || job.status.Status == "partial-failure" || job.status.Status == "cancelled"
	job.mu.Unlock()
	if !terminal {
		job.cancel()
	}
	return job.snapshot(), nil
}

func (s *Service) runUpload(ctx context.Context, id, conflictPolicy string, job *uploadJob) {
	job.setStatus(func(status *UploadStatus) { status.Status = "running" })
	target, err := s.resolveUploadTarget(ctx, id, job.root, job.snapshot().TargetPath)
	if err == nil && conflictPolicy == "refuse" {
		var conflicts []string
		conflicts, err = s.uploadConflicts(ctx, id, job.root, job.snapshot().TargetPath, job.files)
		if len(conflicts) > 0 {
			job.setStatus(func(status *UploadStatus) {
				status.Status = "failed"
				for _, value := range conflicts {
					status.Failures = append(status.Failures, UploadFailure{Path: value, Code: "filesystem_upload_conflict"})
				}
			})
			job.cleanup()
			return
		}
	}
	if err != nil {
		job.fail(err)
		job.cleanup()
		return
	}
	info, err := s.remote.RemoteTransferInfo(id)
	if err != nil {
		job.fail(ErrNoTransport)
		job.cleanup()
		return
	}
	rsAvailable, probeErr := s.rsyncAvailable(ctx, id)
	if probeErr != nil {
		job.fail(ErrNoTransport)
		job.cleanup()
		return
	}
	if rsAvailable {
		if rsync, lookErr := exec.LookPath("rsync"); lookErr == nil {
			job.setStatus(func(status *UploadStatus) { status.Transport = "rsync" })
			err = runRsync(ctx, rsync, info, job, target, conflictPolicy)
			if err != nil {
				s.invalidateRsync(id)
			}
		} else {
			job.setStatus(func(status *UploadStatus) { status.Transport = "scp" })
			err = runScp(ctx, id, info, job, target, conflictPolicy, s)
		}
	} else {
		job.setStatus(func(status *UploadStatus) { status.Transport = "scp" })
		err = runScp(ctx, id, info, job, target, conflictPolicy, s)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		job.setStatus(func(status *UploadStatus) { status.Status = "cancelled" })
	} else if err != nil {
		job.fail(err)
	} else {
		job.setStatus(func(status *UploadStatus) {
			status.Status = "completed"
			status.BytesSent = status.BytesTotal
			status.CurrentPath = ""
		})
	}
	job.cleanup()
}

func (s *Service) resolveUploadTarget(ctx context.Context, id string, root RootContext, relative string) (string, error) {
	result, err := s.remote.ResolveUploadTarget(ctx, id, root.AbsolutePath, relative)
	if err != nil {
		return "", mapRemoteError(err)
	}
	return result, nil
}

func (s *Service) uploadConflicts(ctx context.Context, id string, root RootContext, target string, files []stagedUploadFile) ([]string, error) {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Manifest.RelativePath)
	}
	result, err := s.remote.UploadConflicts(ctx, id, root.AbsolutePath, target, paths)
	if err != nil {
		return nil, mapRemoteError(err)
	}
	return result, nil
}

func (s *Service) rsyncAvailable(ctx context.Context, id string) (bool, error) {
	s.mu.Lock()
	if capability, ok := s.transfers[id]; ok && s.now().Before(capability.ExpiresAt) {
		s.mu.Unlock()
		return capability.Available, nil
	}
	s.mu.Unlock()
	result, err := s.remote.RsyncAvailable(ctx, id)
	if err != nil {
		return false, mapRemoteError(err)
	}
	s.mu.Lock()
	s.transfers[id] = transferCapability{Available: result, ExpiresAt: s.now().Add(30 * time.Second)}
	s.mu.Unlock()
	return result, nil
}

func (s *Service) invalidateRsync(id string) {
	s.mu.Lock()
	delete(s.transfers, id)
	s.mu.Unlock()
}

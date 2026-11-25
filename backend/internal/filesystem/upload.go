package filesystem

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// Upload limits exclude multipart framing.
const (
	MaxUploadBytes = int64(10 << 30)
	MaxUploadFiles = 10_000
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
		if isUploadNotFound(err) {
			return UploadStatus{}, ErrUploadNotFound
		}
		if err != nil {
			return UploadStatus{}, err
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

func (s *Service) runUpload(ctx context.Context, id, conflictPolicy string, job *uploadJob, reservation *uploadReservation) {
	defer reservation.cleanup()
	defer job.cancel()
	job.setStatus(func(status *UploadStatus) { status.Status = "running" })
	conflicts, err := s.transferUpload(ctx, id, conflictPolicy, job)
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		job.setStatus(func(status *UploadStatus) { status.Status = "cancelled" })
	case len(conflicts) > 0:
		job.setStatus(func(status *UploadStatus) {
			status.Status = "failed"
			for _, value := range conflicts {
				status.Failures = append(status.Failures, UploadFailure{Path: value, Code: "filesystem_upload_conflict"})
			}
		})
	case err != nil:
		job.fail(err)
	default:
		job.setStatus(func(status *UploadStatus) {
			status.Status = "completed"
			status.BytesSent = status.BytesTotal
			status.CurrentPath = ""
		})
	}
}

func (s *Service) transferUpload(ctx context.Context, id, conflictPolicy string, job *uploadJob) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relativeTarget := job.snapshot().TargetPath
	target, err := s.resolveUploadTarget(ctx, id, job.root, relativeTarget)
	if err != nil {
		return nil, err
	}
	if conflictPolicy == "refuse" {
		conflicts, err := s.uploadConflicts(ctx, id, job.root, relativeTarget, job.files)
		if len(conflicts) > 0 || err != nil {
			return conflicts, err
		}
	}
	lease, err := s.remote.AcquireRemoteTransfer(ctx, id)
	if err != nil {
		return nil, mapRemoteError(err)
	}
	defer lease.Close()
	info := lease.Info()
	rsAvailable, err := s.rsyncAvailable(ctx, id)
	if err != nil {
		return nil, mapRemoteError(err)
	}
	if rsAvailable {
		if rsync, lookErr := exec.LookPath("rsync"); lookErr == nil {
			job.setStatus(func(status *UploadStatus) { status.Transport = "rsync" })
			err := runRsync(ctx, rsync, info, job, target, conflictPolicy)
			if err != nil {
				s.invalidateRsync(id)
			}
			return nil, err
		}
	}
	job.setStatus(func(status *UploadStatus) { status.Transport = "scp" })
	return nil, runScp(ctx, id, info, job, target, conflictPolicy, s)
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

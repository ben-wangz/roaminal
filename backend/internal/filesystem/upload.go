package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
)

const (
	maxUploadBytes = int64(10 << 30)
	maxUploadFiles = 10_000
)

type uploadJob struct {
	mu         sync.Mutex
	instanceID string
	status     UploadStatus
	staging    string
	files      []stagedUploadFile
	root       RootContext
	cancel     context.CancelFunc
}

type stagedUploadFile struct {
	Manifest UploadManifestFile
	Path     string
}

func (s *Service) UploadStatus(instanceID, uploadID string) (UploadStatus, error) {
	s.mu.Lock()
	job := s.uploads[uploadID]
	s.mu.Unlock()
	if job == nil || job.instanceID != instanceID {
		return UploadStatus{}, ErrUploadNotFound
	}
	return job.snapshot(), nil
}

func (s *Service) CancelUpload(instanceID, uploadID string) (UploadStatus, error) {
	s.mu.Lock()
	job := s.uploads[uploadID]
	s.mu.Unlock()
	if job == nil || job.instanceID != instanceID {
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
	info, err := s.executor.RemoteTransferInfo(id)
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
	result, err := s.executor.RunRemote(ctx, id, connection.RemoteCommand{Script: uploadTargetScript, Args: []string{root.AbsolutePath, relative}, OutputLimit: 16 << 10, Timeout: 5 * time.Second})
	if err != nil {
		return "", mapRemoteError(err)
	}
	parts := strings.Split(string(result.Output), "\x00")
	if len(parts) != 3 || parts[0] != uploadTargetMarker || parts[1] == "" || parts[2] != "" || !strings.HasPrefix(parts[1], "/") {
		return "", ErrProtocol
	}
	return path.Clean(parts[1]), nil
}

func (s *Service) uploadConflicts(ctx context.Context, id string, root RootContext, target string, files []stagedUploadFile) ([]string, error) {
	args := []string{root.AbsolutePath, target}
	for _, file := range files {
		args = append(args, file.Manifest.RelativePath)
	}
	result, err := s.executor.RunRemote(ctx, id, connection.RemoteCommand{Script: uploadConflictScript, Args: args, OutputLimit: 1 << 20, Timeout: 5 * time.Second})
	if err != nil {
		return nil, mapRemoteError(err)
	}
	parts := strings.Split(string(result.Output), "\x00")
	if len(parts) < 3 || parts[0] != uploadConflictMarker || parts[len(parts)-2] != uploadConflictEnd || parts[len(parts)-1] != "" {
		return nil, ErrProtocol
	}
	return append([]string(nil), parts[1:len(parts)-2]...), nil
}

func (s *Service) rsyncAvailable(ctx context.Context, id string) (bool, error) {
	s.mu.Lock()
	if capability, ok := s.transfers[id]; ok && s.now().Before(capability.ExpiresAt) {
		s.mu.Unlock()
		return capability.Available, nil
	}
	s.mu.Unlock()
	result, err := s.executor.RunRemote(ctx, id, connection.RemoteCommand{Script: rsyncProbeScript, OutputLimit: 64, Timeout: 2 * time.Second})
	if err != nil {
		return false, mapRemoteError(err)
	}
	value := strings.TrimSpace(string(result.Output)) == rsyncAvailableMarker
	s.mu.Lock()
	s.transfers[id] = transferCapability{Available: value, ExpiresAt: s.now().Add(30 * time.Second)}
	s.mu.Unlock()
	return value, nil
}

func (s *Service) invalidateRsync(id string) {
	s.mu.Lock()
	delete(s.transfers, id)
	s.mu.Unlock()
}

func (j *uploadJob) snapshot() UploadStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	value := j.status
	value.Failures = append([]UploadFailure(nil), j.status.Failures...)
	return value
}

func (j *uploadJob) setStatus(update func(*UploadStatus)) {
	j.mu.Lock()
	update(&j.status)
	j.mu.Unlock()
}

func (j *uploadJob) fail(err error) {
	code := "filesystem_upload_failed"
	if errors.Is(err, ErrNoTransport) {
		code = "filesystem_upload_transport_unavailable"
	}
	j.setStatus(func(status *UploadStatus) {
		if status.Status != "partial-failure" {
			status.Status = "failed"
		}
		status.Failures = append(status.Failures, UploadFailure{Code: code, Error: err.Error()})
	})
}

func (j *uploadJob) cleanup() {
	_ = os.RemoveAll(j.staging)
}

func pathErrOrInvalid(err error) error {
	if err == nil {
		return ErrInvalidPath
	}
	return err
}

func newUploadID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(value[:])
}

func remoteSpec(alias, remotePath string) string {
	return alias + ":" + shellQuote(remotePath)
}

func sshTransportCommand(info connection.RemoteTransferInfo) string {
	return shellQuote(info.SSHPath) + " -T -o ControlMaster=no -o ControlPersist=no -o " + shellQuote("ControlPath="+info.ControlPath) + " -o BatchMode=yes -o ClearAllForwardings=yes --"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

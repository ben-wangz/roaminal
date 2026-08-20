package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

func (s *Service) CreateUpload(ctx context.Context, id string, manifest UploadManifest, parts map[string]*multipart.FileHeader) (UploadStatus, error) {
	if s.executor == nil {
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
	staging, err := os.MkdirTemp("", "roaminal-filesystem-upload-")
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
	uploadID := newUploadID()
	jobContext, cancel := context.WithCancel(context.Background())
	job := &uploadJob{
		instanceID: id,
		status:     UploadStatus{UploadID: uploadID, Status: "queued", Transport: "pending", TargetPath: target, BytesTotal: total, Failures: []UploadFailure{}},
		staging:    staging,
		files:      files,
		root:       root,
		cancel:     cancel,
	}
	s.mu.Lock()
	s.uploads[uploadID] = job
	s.mu.Unlock()
	cleanup = false
	go s.runUpload(jobContext, id, manifest.ConflictPolicy, job)
	return job.snapshot(), nil
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

type transferCapability struct {
	Available bool
	ExpiresAt time.Time
}

func runRsync(ctx context.Context, binary string, info connection.RemoteTransferInfo, job *uploadJob, target, conflictPolicy string) error {
	args := []string{"-a", "--partial", "--protect-args", "--info=progress2", "-e", sshTransportCommand(info)}
	if conflictPolicy == "update-if-newer" {
		args = append(args, "--update")
	}
	args = append(args, filepath.Join(job.staging, "."), remoteSpec(info.Alias, target))
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdout = &rsyncProgressWriter{job: job}
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return ErrUploadFailed
	}
	return nil
}

type rsyncProgressWriter struct {
	job     *uploadJob
	partial string
}

func (w *rsyncProgressWriter) Write(value []byte) (int, error) {
	w.partial += string(value)
	for {
		index := strings.IndexAny(w.partial, "\r\n")
		if index < 0 {
			if len(w.partial) > 4096 {
				w.partial = w.partial[len(w.partial)-4096:]
			}
			break
		}
		w.update(w.partial[:index])
		w.partial = strings.TrimLeft(w.partial[index+1:], "\r\n")
	}
	return len(value), nil
}

func (w *rsyncProgressWriter) update(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	value, err := strconv.ParseInt(strings.ReplaceAll(fields[0], ",", ""), 10, 64)
	if err != nil || value < 0 {
		return
	}
	w.job.setStatus(func(status *UploadStatus) {
		if value > status.BytesTotal {
			value = status.BytesTotal
		}
		if value > status.BytesSent {
			status.BytesSent = value
		}
	})
}

func runScp(ctx context.Context, id string, info connection.RemoteTransferInfo, job *uploadJob, target, conflictPolicy string, service *Service) error {
	if err := service.createRemoteDirectories(ctx, id, info, job.root.AbsolutePath, job.snapshot().TargetPath, job.files); err != nil {
		return err
	}
	scp, err := exec.LookPath("scp")
	if err != nil {
		return ErrUploadFailed
	}
	for _, file := range job.files {
		if conflictPolicy == "update-if-newer" {
			upload, checkErr := service.shouldUploadWithScp(ctx, id, target, file.Manifest)
			if checkErr != nil {
				return checkErr
			}
			if !upload {
				continue
			}
		}
		job.setStatus(func(status *UploadStatus) { status.CurrentPath = file.Manifest.RelativePath })
		args := []string{"-p", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + info.ControlPath, "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "--", file.Path, remoteSpec(info.Alias, path.Join(target, file.Manifest.RelativePath))}
		command := exec.CommandContext(ctx, scp, args...)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			job.setStatus(func(status *UploadStatus) {
				status.Status = "partial-failure"
				status.Failures = append(status.Failures, UploadFailure{Path: file.Manifest.RelativePath, Code: "filesystem_upload_failed"})
			})
			return ErrUploadFailed
		}
		job.setStatus(func(status *UploadStatus) { status.BytesSent += file.Manifest.Size })
	}
	return nil
}

func (s *Service) createRemoteDirectories(ctx context.Context, id string, _ connection.RemoteTransferInfo, root, target string, files []stagedUploadFile) error {
	directories := make([]string, 0, len(files))
	seen := make(map[string]bool)
	for _, file := range files {
		directory := path.Dir(file.Manifest.RelativePath)
		for directory != "." {
			if !seen[directory] {
				seen[directory] = true
				directories = append(directories, directory)
			}
			directory = path.Dir(directory)
		}
	}
	if len(directories) == 0 {
		return nil
	}
	args := []string{root, target}
	args = append(args, directories...)
	if _, err := s.executor.RunRemote(ctx, id, connection.RemoteCommand{Script: remoteMkdirScript, Args: args, OutputLimit: 1024, Timeout: 10 * time.Second}); err != nil {
		return mapRemoteError(err)
	}
	return nil
}

func (s *Service) shouldUploadWithScp(ctx context.Context, id, target string, file UploadManifestFile) (bool, error) {
	result, err := s.executor.RunRemote(ctx, id, connection.RemoteCommand{Script: remoteMtimeScript, Args: []string{target, file.RelativePath, strconv.FormatInt(file.ModifiedAt.Unix(), 10)}, OutputLimit: 64, Timeout: 5 * time.Second})
	if err != nil {
		return false, mapRemoteError(err)
	}
	value := strings.TrimSpace(string(result.Output))
	if value != "upload" && value != "skip" {
		return false, ErrProtocol
	}
	return value == "upload", nil
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

const (
	rsyncAvailableMarker = "ROAMINAL_RSYNC_AVAILABLE"
	uploadTargetMarker   = "ROAMINAL_FILESYSTEM_UPLOAD_TARGET_V1"
	uploadConflictMarker = "ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_V1"
	uploadConflictEnd    = "ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_END"
)

const rsyncProbeScript = `if command -v rsync >/dev/null 2>&1; then printf '%s' 'ROAMINAL_RSYNC_AVAILABLE'; else printf '%s' 'ROAMINAL_RSYNC_UNAVAILABLE'; fi`

const uploadTargetScript = `set -eu
root=$1
relative=$2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
target_real=$(cd -- "$target" && pwd -P)
case "$target_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
test -d "$target_real"
printf '%s\0%s\0' 'ROAMINAL_FILESYSTEM_UPLOAD_TARGET_V1' "$target_real"
`

const uploadConflictScript = `set -eu
root=$1
relative=$2
shift 2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
target_real=$(cd -- "$target" && pwd -P)
case "$target_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
printf '%s\0' 'ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_V1'
for file in "$@"; do
  candidate=$target_real/$file
  if [ -e "$candidate" ] || [ -L "$candidate" ]; then printf '%s\0' "$file"; fi
done
printf '%s\0' 'ROAMINAL_FILESYSTEM_UPLOAD_CONFLICTS_END'
`

const remoteMkdirScript = `set -eu
root=$1
relative=$2
shift 2
root_real=$(cd -- "$root" && pwd -P)
target=$root_real
if [ "$relative" != "." ]; then target=$root_real/$relative; fi
target_real=$(cd -- "$target" && pwd -P)
case "$target_real" in
  "$root_real"|"$root_real"/*) ;;
  *) exit 23 ;;
esac
for directory in "$@"; do
  mkdir -p -- "$target_real/$directory"
done
`

const remoteMtimeScript = `set -eu
target=$1
relative=$2
local_mtime=$3
candidate=$target/$relative
if [ ! -e "$candidate" ] && [ ! -L "$candidate" ]; then printf '%s' upload; exit 0; fi
if [ "$local_mtime" -le 0 ]; then printf '%s' upload; exit 0; fi
remote_mtime=$(stat -c '%Y' -- "$candidate" 2>/dev/null || stat -f '%m' "$candidate" 2>/dev/null || printf '%s' 0)
case "$remote_mtime" in
  ''|*[!0-9-]*) printf '%s' upload ;;
  *) if [ "$remote_mtime" -ge "$local_mtime" ]; then printf '%s' skip; else printf '%s' upload; fi ;;
esac
`

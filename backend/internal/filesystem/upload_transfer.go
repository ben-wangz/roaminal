package filesystem

import (
	"context"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type transferCapability struct {
	Available bool
	ExpiresAt time.Time
}

func runRsync(ctx context.Context, binary string, info ports.RemoteTransferInfo, job *uploadJob, target, conflictPolicy string) error {
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

func runScp(ctx context.Context, id string, info ports.RemoteTransferInfo, job *uploadJob, target, conflictPolicy string, service *Service) error {
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
		args := []string{"-p", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + info.ControlPath, "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "--", file.Path, scpRemoteSpec(info.Alias, path.Join(target, file.Manifest.RelativePath))}
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

func (s *Service) createRemoteDirectories(ctx context.Context, id string, _ ports.RemoteTransferInfo, root, target string, files []stagedUploadFile) error {
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
	if err := s.remote.CreateDirectories(ctx, id, root, target, directories); err != nil {
		return mapRemoteError(err)
	}
	return nil
}

func (s *Service) shouldUploadWithScp(ctx context.Context, id, target string, file UploadManifestFile) (bool, error) {
	result, err := s.remote.ShouldUploadWithScp(ctx, id, target, file.RelativePath, file.ModifiedAt.Unix())
	if err != nil {
		return false, mapRemoteError(err)
	}
	return result, nil
}

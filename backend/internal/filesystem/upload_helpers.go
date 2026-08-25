package filesystem

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

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
	if j.persist != nil {
		j.persist()
	}
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
	if j.persist != nil {
		j.persist()
	}
}

func pathErrOrInvalid(err error) error {
	if err == nil {
		return ErrInvalidPath
	}
	return err
}

func (s *Service) newUploadID() string {
	var value [16]byte
	if s.random != nil {
		if _, err := s.random.Read(value[:]); err == nil {
			return base64.RawURLEncoding.EncodeToString(value[:])
		}
	}
	return fmt.Sprintf("upload-%x", s.now().UnixNano())
}

func remoteSpec(alias, remotePath string) string {
	return alias + ":" + shellQuote(remotePath)
}

// scpRemoteSpec is passed directly to scp. Unlike rsync's remote-shell
// protocol, OpenSSH scp's default SFTP mode does not remove shell quotes from
// the path, so quoting here would make the quotes part of the remote filename.
func scpRemoteSpec(alias, remotePath string) string {
	return alias + ":" + remotePath
}

func sshTransportCommand(info ports.RemoteTransferInfo) string {
	return shellQuote(info.SSHPath) + " -T -o ControlMaster=no -o ControlPersist=no -o " + shellQuote("ControlPath="+info.ControlPath) + " -o BatchMode=yes -o ClearAllForwardings=yes --"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

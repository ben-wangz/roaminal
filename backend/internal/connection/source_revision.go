package connection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os/exec"
	"time"
)

const sourceRevisionOutputLimit = 1 << 20

// sourceRevision fingerprints the effective OpenSSH configuration for one
// alias. The global SSH config ETag is intentionally not used here: changing
// one Host block must not invalidate transports owned by other aliases.
func (m *Manager) sourceRevision(alias string) (string, error) {
	if m.sshPath == "" || alias == "" {
		return "", errors.New("ssh source fingerprint is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.sshPath,
		"-G",
		"-o", "BatchMode=yes",
		"-o", "ControlPath=none",
		"--", alias,
	)
	writer := &boundedHashWriter{hash: sha256.New(), limit: sourceRevisionOutputLimit}
	cmd.Stdout = writer
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh effective configuration for %q: %w", alias, err)
	}
	if writer.exceeded {
		return "", errors.New("ssh effective configuration exceeds size limit")
	}
	return hex.EncodeToString(writer.hash.Sum(nil)), nil
}

type boundedHashWriter struct {
	hash     hash.Hash
	limit    int
	count    int
	exceeded bool
}

func (w *boundedHashWriter) Write(data []byte) (int, error) {
	if w.count+len(data) > w.limit {
		w.exceeded = true
		return 0, errors.New("output exceeds limit")
	}
	w.count += len(data)
	return w.hash.Write(data)
}

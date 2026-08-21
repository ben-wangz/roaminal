package report

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return os.Mkdir(path, 0700)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("private spool directory permissions are unsafe")
	}
	return nil
}

type senderLockOwner struct {
	PID          int    `json:"pid"`
	ProcessStart string `json:"processStart"`
	CreatedAt    string `json:"createdAt"`
}

func staleSenderLock(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 || time.Since(info.ModTime()) < 2*time.Minute {
		return false
	}
	data, err := os.ReadFile(filepath.Join(path, "owner"))
	if err != nil {
		return true
	}
	var owner senderLockOwner
	if json.Unmarshal(data, &owner) != nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return true
		}
		owner.PID = pid
	}
	if owner.PID < 1 {
		return true
	}
	if !tmux.ProcessAlive(owner.PID) {
		return true
	}
	return owner.ProcessStart != "" && tmux.ProcessStart(owner.PID) != "" && owner.ProcessStart != tmux.ProcessStart(owner.PID)
}

func waitRetry(ctx context.Context) error {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

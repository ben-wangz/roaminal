package worker

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

const processGroupStopGrace = 2 * time.Second

func terminateProcessGroup(ctx context.Context, cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	grace, cancel := context.WithTimeout(ctx, processGroupStopGrace)
	defer cancel()
	for processGroupAlive(cmd) {
		select {
		case <-grace.Done():
			goto force
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil
force:
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func processGroupAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}

package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const sessionProcessGroupGrace = 2 * time.Second

func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions)+len(m.pending))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	for _, session := range m.pending {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	stopDone := make(chan struct{}, len(sessions))
	for _, session := range sessions {
		go func(session *Session) {
			session.mu.Lock()
			cmd := session.cmd
			session.closed = true
			session.meta.Lifecycle = "interrupted"
			session.meta.BackendRuntimeID = session.manager.runtimeID
			session.meta.UpdatedAt = time.Now().UTC()
			if session.snapshotTimer != nil {
				session.snapshotTimer.Stop()
				session.snapshotTimer = nil
			}
			if session.pty != nil {
				_ = session.pty.Close()
			}
			session.mu.Unlock()
			_ = terminateSessionProcessGroup(ctx, cmd)
			if onExit := session.takeExitHook(); onExit != nil {
				onExit(ExitStatus{})
			}
			var err error
			if session.ephemeral {
				m.finishEphemeral(ctx, session)
			} else {
				err = m.retireSession(ctx, session)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Roaminal session %s shutdown cleanup warning: %v\n", session.meta.ID, err)
			}
			stopDone <- struct{}{}
		}(session)
	}
	for range sessions {
		<-stopDone
	}
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if m.worker != nil {
		_ = m.worker.Shutdown(deadline)
	}
}

func signalProcessGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, signal)
}
func terminateSessionProcessGroup(ctx context.Context, cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := signalProcessGroup(cmd, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	grace, cancel := context.WithTimeout(ctx, sessionProcessGroupGrace)
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
	if err := signalProcessGroup(cmd, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
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

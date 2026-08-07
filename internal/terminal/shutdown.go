package terminal

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.mu.Lock()
		if session.snapshotTimer != nil {
			session.snapshotTimer.Stop()
		}
		session.mu.Unlock()
		session.saveSnapshot()
		session.mu.Lock()
		if !session.closed {
			_ = signalProcessGroup(session.cmd, syscall.SIGTERM)
			_ = session.pty.Close()
		}
		session.mu.Unlock()
	}
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = m.worker.Shutdown(deadline)
}
func signalProcessGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, signal)
}

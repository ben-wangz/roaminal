package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
)

const sessionProcessGroupGrace = 2 * time.Second

func (m *Manager) Shutdown(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, config.DefaultShutdownDeadline)
	defer cancel()
	deadline, _ := ctx.Deadline()
	// Leave some time for worker shutdown even if all retirement stages stall.
	reserve := min(time.Until(deadline)/4, time.Second)
	if reserve < 0 {
		reserve = 0
	}
	retireCtx, stopRetirement := context.WithDeadline(ctx, deadline.Add(-reserve))
	defer stopRetirement()

	m.mu.Lock()
	m.stopping = true
	m.stopCreating()
	sessions := make(map[string]*Session, len(m.sessions)+len(m.pending))
	for id, session := range m.sessions {
		sessions[id] = session
	}
	for id, session := range m.pending {
		sessions[id] = session
	}
	m.mu.Unlock()

	var retirements sync.WaitGroup
	for id, session := range sessions {
		retirements.Go(func() {
			if err := m.terminateSession(retireCtx, session, "interrupted"); err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "Roaminal connection instance %s shutdown cleanup warning: %v\n", id, err)
			}
		})
	}
	done := make(chan struct{})
	go func() { retirements.Wait(); close(done) }()
	select {
	case <-done:
	case <-retireCtx.Done():
	}
	// A periodic save or an earlier natural retirement can still be in flight.
	// Cancel those operations too and prevent any late worker from writing.
	m.storage.stop()
	if m.worker != nil {
		workerCtx, stopWorker := context.WithTimeout(ctx, 5*time.Second)
		_ = m.worker.Shutdown(workerCtx)
		stopWorker()
	}
	<-done
	m.creates.Wait()
	// Some storage calls (e.g. a SQLite fsync) can outlive context cancellation.
	// Do not return to the store/state-lock owner while a writer is still active.
	m.storage.inFlight.Wait()
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

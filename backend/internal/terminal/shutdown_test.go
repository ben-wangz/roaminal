package terminal

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestShutdownSignalsProcessDespiteStalledSessionLock(t *testing.T) {
	m, _, meta := retirementFixture(t)
	worker := shutdownWorker{stopped: make(chan struct{})}
	m.worker = worker
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		waitForCleanup(t, exited)
	})
	session := &Session{manager: m, meta: meta, cmd: cmd}
	m.sessions[meta.ID] = session
	session.mu.Lock()
	defer session.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	returned := make(chan struct{})
	go func() { m.Shutdown(ctx); close(returned) }()
	waitForCleanup(t, returned)
	waitForCleanup(t, worker.stopped)
	waitForCleanup(t, exited)
}

func TestTerminateSessionProcessGroupEscalatesAndReapsChildren(t *testing.T) {
	cmd := exec.Command("/bin/bash", "-c", "sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := terminateSessionProcessGroup(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("process group did not terminate")
	}
}

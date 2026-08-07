package terminal

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

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

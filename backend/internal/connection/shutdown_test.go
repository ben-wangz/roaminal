package connection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type shutdownTestRuntime struct {
	ports.TerminalRuntime
	started chan error
}

func (r shutdownTestRuntime) Shutdown(ctx context.Context) { r.started <- ctx.Err() }

func TestShutdownDoesNotWaitForMuxBeforeTerminal(t *testing.T) {
	m, transport, closeFixture := newReadyTransportManager(t)
	defer closeFixture()
	if err := os.WriteFile(m.sshPath, []byte("#!/bin/sh\nexec sleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := shutdownTestRuntime{started: make(chan error, 1)}
	m.instances = NewInstanceService(runtime)
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { m.Shutdown(ctx); close(done) }()
	select {
	case err := <-runtime.started:
		if err != nil {
			t.Fatalf("mux cleanup consumed the terminal shutdown budget: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("terminal shutdown did not start before its deadline")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel the stalled SSH control command")
	}
	if _, err := os.Stat(filepath.Dir(transport.ControlPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SSH control directory was not removed: %v", err)
	}
}

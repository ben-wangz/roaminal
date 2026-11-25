package worker

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type observedPipeWriter struct {
	*io.PipeWriter
	started chan struct{}
	once    sync.Once
}

func (w *observedPipeWriter) Write(payload []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	return w.PipeWriter.Write(payload)
}

func waitWorkerTest[T any](t *testing.T, done <-chan T) T {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("worker operation stalled")
		panic("unreachable")
	}
}

func stoppedPipeReader(t *testing.T) (*Client, <-chan struct{}, <-chan struct{}) {
	t.Helper()
	reader, writer := io.Pipe()
	c := New("unused", nil)
	observed := &observedPipeWriter{PipeWriter: writer, started: make(chan struct{})}
	c.stdin = observed
	c.writeQueue = make(chan writeRequest, 1)
	stopped := make(chan struct{})
	go func() { c.writeLoop(); close(stopped) }()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
		c.fail(ErrUnavailable)
		waitWorkerTest(t, stopped)
	})
	return c, observed.started, stopped
}

func TestSnapshotRequestHonorsDeadlineDuringFrameWrite(t *testing.T) {
	c, started, _ := stoppedPipeReader(t)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := c.Snapshot(ctx, "terminal", "1")
		done <- err
	}()
	waitWorkerTest(t, started)
	if err := waitWorkerTest(t, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting for frame write returned %v", err)
	}
	c.waitMu.Lock()
	defer c.waitMu.Unlock()
	if len(c.waiters) != 0 || !c.Available() {
		t.Fatal("request timeout leaked a waiter or failed the worker")
	}
}

func TestWorkerShutdownUnblocksStalledWriter(t *testing.T) {
	c, started, writerStopped := stoppedPipeReader(t)
	writeDone := make(chan error, 1)
	go func() { writeDone <- c.Write("terminal", "1", []byte("fixture")) }()
	waitWorkerTest(t, started)
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	shutdown := make(chan error, 1)
	go func() { shutdown <- c.Shutdown(ctx) }()
	if err := waitWorkerTest(t, shutdown); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown with stalled pipe returned %v", err)
	}
	if err := waitWorkerTest(t, writeDone); err == nil {
		t.Fatal("stalled write succeeded after worker shutdown")
	}
	waitWorkerTest(t, writerStopped)
	if c.Available() {
		t.Fatal("worker remained available after shutdown")
	}
}

func TestWorkerDropsCancelledRequestBeforeStartingFrame(t *testing.T) {
	c, started, _ := stoppedPipeReader(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request := writeRequest{ctx: ctx, data: []byte("not sent"), queueSize: 1, done: make(chan error, 1)}
	c.queuedBytes = int64(request.queueSize)
	c.writeQueue <- request
	if err := waitWorkerTest(t, request.done); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled queued request returned %v", err)
	}
	select {
	case <-started:
		t.Fatal("worker wrote a frame already cancelled while queued")
	default:
	}
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if c.queuedBytes != 0 {
		t.Fatalf("cancelled request leaked queue bytes: %d", c.queuedBytes)
	}
}

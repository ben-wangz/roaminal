package connection

import (
	"io"
	"os/exec"
	"sync"
)

type auxiliaryReader struct {
	manager   *Manager
	transport *Transport
	stdout    io.ReadCloser
	stderr    *cappedBuffer
	cmd       *exec.Cmd
	mu        sync.Mutex
	finished  bool
	waitErr   error
	cancel    func()
	done      chan struct{}
}

func (r *auxiliaryReader) Read(p []byte) (int, error) {
	n, err := r.stdout.Read(p)
	if err != nil {
		r.finish()
	}
	return n, err
}

func (r *auxiliaryReader) Close() error {
	if !r.finishedState() {
		_ = r.stdout.Close()
		if r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
		}
	}
	r.finish()
	r.mu.Lock()
	err := r.waitErr
	r.mu.Unlock()
	return err
}

func (r *auxiliaryReader) finishedState() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished
}

func (r *auxiliaryReader) errorOutput() []byte {
	if r.stderr == nil {
		return nil
	}
	return append([]byte(nil), r.stderr.data.Bytes()...)
}

func (r *auxiliaryReader) finish() {
	r.mu.Lock()
	if r.finished {
		done := r.done
		r.mu.Unlock()
		<-done
		return
	}
	r.finished = true
	r.mu.Unlock()
	err := classifyAuxiliaryError(r.cmd.Wait(), r.errorOutput())
	if r.cancel != nil {
		r.cancel()
	}
	r.manager.releaseAuxiliary(r.transport)
	r.mu.Lock()
	r.waitErr = err
	close(r.done)
	r.mu.Unlock()
}

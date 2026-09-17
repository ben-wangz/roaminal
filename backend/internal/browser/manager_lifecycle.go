package browser

import (
	"context"
	"syscall"
)

func (m *Manager) Shutdown(_ context.Context) {
	m.mu.Lock()
	m.shutdown = true
	runtime := m.process
	m.process = nil
	for current := range m.viewers {
		current.closeOne.Do(func() { close(current.done) })
	}
	m.viewers = make(map[*viewer]struct{})
	m.primary = ""
	m.primaryKnown = false
	m.mu.Unlock()
	if runtime != nil && runtime.cmd.Process != nil {
		_ = syscall.Kill(-runtime.cmd.Process.Pid, syscall.SIGTERM)
	}
}

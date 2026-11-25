package connection

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/processenv"
)

func (m *Manager) Shutdown(ctx context.Context) {
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.transportPool.mu.Lock()
	transports := make([]*Transport, 0, len(m.transportPool.transports))
	for _, transport := range m.transportPool.transports {
		transports = append(transports, transport)
	}
	m.transportPool.transports = make(map[string]*Transport)
	m.transportPool.instances = make(map[string]*Transport)
	m.transportPool.mu.Unlock()
	if m.remoteMonitor != nil {
		m.remoteMonitor.Close()
	}
	// A stalled OpenSSH control command must not use up the PTY retirement
	// budget. Both paths share the deadline and finish before we return.
	var stops sync.WaitGroup
	for _, transport := range transports {
		stops.Go(func() { m.stopTransport(ctx, transport) })
	}
	m.instances.Shutdown(ctx)
	stops.Wait()
}

func (m *Manager) stopTransport(parent context.Context, transport *Transport) {
	if m.sshPath != "" {
		ctx, cancel := context.WithTimeout(parent, 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, m.sshPath, "-F", "none", "-S", transport.ControlPath, "-O", "exit", "--", transport.Alias)
		cmd.Env = processenv.Environment()
		_ = cmd.Run()
	}
	_ = os.RemoveAll(filepath.Dir(transport.ControlPath))
}

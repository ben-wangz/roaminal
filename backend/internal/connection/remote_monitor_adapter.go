package connection

import (
	"context"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// Target resolves a live connection instance to an opaque monitor target.
// The monitor feature never receives the SSH transport itself.
func (m *Manager) Target(ctx context.Context, id string) (ports.MonitorTarget, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ports.MonitorTarget{}, ctx.Err()
		default:
		}
	}
	transport, err := m.remoteTransport(id)
	if err != nil {
		return ports.MonitorTarget{}, err
	}
	return ports.MonitorTarget{OwnerID: transport.OwnerID}, nil
}

// Probe is the SSH/tmux infrastructure adapter for the monitor feature.
// Script framing and parsing stay in internal/monitor; this method only
// performs the auxiliary remote execution through the reusable transport.
func (m *Manager) Probe(ctx context.Context, target ports.MonitorTarget, request ports.MonitorProbeRequest) (ports.MonitorProbeResult, error) {
	if target.OwnerID == "" || request.Script == "" || request.Nonce == "" {
		return ports.MonitorProbeResult{}, ports.ErrRemoteNoTransport
	}
	transport, err := m.remoteTransport(target.OwnerID)
	if err != nil {
		return ports.MonitorProbeResult{}, err
	}
	output, err := m.runAuxiliaryInput(ctx, transport, strings.NewReader(request.Script), "sh", "-s", "--", request.Nonce)
	if err != nil {
		return ports.MonitorProbeResult{}, err
	}
	return ports.MonitorProbeResult{Output: output}, nil
}

func (m *Manager) RemoteMonitor(ctx context.Context, id string) (monitor.RemoteMonitorSnapshot, error) {
	if m.remoteMonitor == nil {
		return monitor.RemoteMonitorSnapshot{}, ports.ErrRemoteNoTransport
	}
	return m.remoteMonitor.RemoteMonitor(ctx, id)
}

func (m *Manager) remoteTransport(id string) (*Transport, error) {
	var summary Summary
	for _, item := range m.Summaries() {
		if item.ID == id {
			summary = item
			break
		}
	}
	if summary.ID == "" {
		return nil, ports.ErrRemoteInstanceNotFound
	}
	if summary.Type != "ssh" || summary.Lifecycle != "live" {
		return nil, ports.ErrRemoteNoTransport
	}
	m.transportPool.mu.Lock()
	transport := m.transportPool.instances[id]
	if transport != nil && transport.OwnerID != id {
		transport = m.transportPool.transports[transport.OwnerID]
	}
	if !transportAcceptsReuse(transport) {
		transport = nil
	}
	m.transportPool.mu.Unlock()
	if transport == nil {
		return nil, ports.ErrRemoteNoTransport
	}
	return transport, nil
}

func (m *Manager) clearRemoteState(ownerID string) {
	if m.remoteMonitor != nil {
		m.remoteMonitor.Clear(ownerID)
	}
}

var _ ports.MonitorProbe = (*Manager)(nil)

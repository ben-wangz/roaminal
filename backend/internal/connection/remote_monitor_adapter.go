package connection

import (
	"context"
	"log"
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
	transport, err := m.ownerTransport(target.OwnerID)
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
	transport, summary, err := m.lookupRemoteTransport(id)
	if err != nil {
		return nil, err
	}
	if !m.transportReady(transport) {
		logRemoteTransportUnavailable(id, summary, "control_socket_unavailable")
		return nil, ports.ErrTransportUnavailable
	}
	return transport, nil
}

// auxiliaryTransport resolves an instance and pins its transport before
// checking the socket. The pin closes the owner-exit cleanup race: once the
// reservation succeeds, finishInstance cannot remove the mux directory until
// the auxiliary command releases its channel.
func (m *Manager) auxiliaryTransport(ctx context.Context, id string) (*Transport, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	transport, summary, err := m.lookupRemoteTransport(id)
	if err != nil {
		return nil, err
	}
	if !m.acquireAuxiliary(ctx, transport) {
		logRemoteTransportUnavailable(id, summary, "control_socket_unavailable")
		return nil, ports.ErrTransportUnavailable
	}
	return transport, nil
}

func (m *Manager) lookupRemoteTransport(id string) (*Transport, Summary, error) {
	var summary Summary
	for _, item := range m.Summaries() {
		if item.ID == id || item.ConnectionInstanceID == id {
			summary = item
			break
		}
	}
	if summary.ID == "" {
		return nil, Summary{}, ports.ErrRemoteInstanceNotFound
	}
	if summary.Type != "ssh" || summary.Lifecycle != "live" {
		return nil, summary, ports.ErrRemoteNoTransport
	}
	if m.transportPool == nil {
		logRemoteTransportUnavailable(id, summary, "mapping_missing")
		return nil, summary, ports.ErrTransportUnavailable
	}
	m.transportPool.mu.Lock()
	lookupID := summary.ID
	transport := m.transportPool.instances[lookupID]
	if transport != nil && transport.OwnerID != lookupID {
		transport = m.transportPool.transports[transport.OwnerID]
	}
	if !transportAcceptsAuxiliary(transport) {
		transport = nil
	}
	m.transportPool.mu.Unlock()
	if transport == nil {
		logRemoteTransportUnavailable(id, summary, "mapping_missing")
		return nil, summary, ports.ErrTransportUnavailable
	}
	return transport, summary, nil
}

// ownerTransport resolves the opaque monitor owner directly. A derived
// connection instance may outlive its owner, so Probe must not require the
// owner to remain in the user-visible instance summary.
func (m *Manager) ownerTransport(ownerID string) (*Transport, error) {
	if m.transportPool == nil {
		return nil, ports.ErrTransportUnavailable
	}
	m.transportPool.mu.Lock()
	transport := m.transportPool.transports[ownerID]
	if !transportAcceptsAuxiliary(transport) {
		transport = nil
	}
	m.transportPool.mu.Unlock()
	if transport == nil {
		return nil, ports.ErrTransportUnavailable
	}
	return transport, nil
}

func logRemoteTransportUnavailable(id string, summary Summary, reason string) {
	log.Printf("remote_transport_unavailable instance_id=%q source_alias=%q lifecycle=%q source_state=%q reason=%q", id, valueOrEmpty(summary.SourceHostAlias), summary.Lifecycle, summary.SourceState, reason)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (m *Manager) clearRemoteState(ownerID string) {
	if m.remoteMonitor != nil {
		m.remoteMonitor.Clear(ownerID)
	}
}

var _ ports.MonitorProbe = (*Manager)(nil)

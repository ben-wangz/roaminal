package connection

import (
	"context"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// The connection service exposes only the terminal capabilities needed by
// adapters and HTTP handlers. The runtime remains an implementation detail of
// the connection aggregate instead of an anonymously embedded manager.
func (m *Manager) Fatal() <-chan error { return m.instances.Fatal() }

func (m *Manager) WorkerFatal(err error) { m.instances.WorkerFatal(err) }

func (m *Manager) PersistenceDegraded() bool { return m.instances.PersistenceDegraded() }

func (m *Manager) InitialCwd() string { return m.instances.InitialCwd() }

func (m *Manager) RuntimeID() string { return m.instances.RuntimeID() }

func (m *Manager) Summaries() []Summary {
	result := m.instances.Summaries()
	for index := range result {
		result[index].RemoteCapability = m.remoteCapability(result[index])
	}
	return result
}

func (m *Manager) remoteCapability(summary Summary) ports.RemoteCapability {
	if summary.Type != "ssh" || summary.Purpose != "interactive" {
		return ports.RemoteCapability{Status: "unsupported", Reason: "remote auxiliary features require an interactive SSH connection instance"}
	}
	if summary.Lifecycle != "live" {
		return ports.RemoteCapability{Status: "unavailable", Reason: "connection instance is not live"}
	}
	if m.transportPool == nil {
		return ports.RemoteCapability{Status: "transport_unavailable", Retryable: true, Reason: "SSH control transport pool is unavailable"}
	}
	m.transportPool.mu.Lock()
	transport := m.transportPool.instances[summary.ID]
	if transport != nil && transport.OwnerID != summary.ID {
		transport = m.transportPool.transports[transport.OwnerID]
	}
	if transport == nil {
		m.transportPool.mu.Unlock()
		return ports.RemoteCapability{Status: "transport_unavailable", Retryable: true, Reason: "SSH control transport is unavailable"}
	}
	if transport.Draining {
		state := transport.SourceState
		m.transportPool.mu.Unlock()
		switch state {
		case "deleted":
			return ports.RemoteCapability{Status: "source_deleted", Reason: "the SSH host definition was deleted; existing channels remain usable until they exit"}
		default:
			return ports.RemoteCapability{Status: "source_stale", Reason: "the SSH host definition changed; existing channels remain usable"}
		}
	}
	if !transportAcceptsAuxiliary(transport) {
		m.transportPool.mu.Unlock()
		return ports.RemoteCapability{Status: "transport_unavailable", Retryable: true, Reason: "SSH control transport is unavailable"}
	}
	m.transportPool.mu.Unlock()
	return ports.RemoteCapability{Status: "available"}
}

func (m *Manager) Create(ctx context.Context, cwd string, cols, rows int) (Summary, error) {
	return m.instances.Create(ctx, cwd, cols, rows)
}

func (m *Manager) AttachReserved(ctx context.Context, id string) (ports.TerminalClient, error) {
	return m.instances.AttachReserved(ctx, id)
}

func (m *Manager) AttachPendingReserved(ctx context.Context, id string) (ports.TerminalClient, error) {
	return m.instances.AttachPendingReserved(ctx, id)
}

func (m *Manager) ReserveAttach(id string) error { return m.instances.ReserveAttach(id) }

func (m *Manager) ReservePendingAttach(id string) error { return m.instances.ReservePendingAttach(id) }

func (m *Manager) ReleaseAttach(id string) { m.instances.ReleaseAttach(id) }

func (m *Manager) ReleasePendingAttach(id string) { m.instances.ReleasePendingAttach(id) }

func (m *Manager) Detach(id string, client ports.TerminalClient) { m.instances.Detach(id, client) }

func (m *Manager) DetachPending(id string, client ports.TerminalClient) {
	m.instances.DetachPending(id, client)
}

func (m *Manager) Input(id string, client ports.TerminalClient, data string) error {
	return m.instances.Input(id, client, data)
}

func (m *Manager) InputPending(id string, client ports.TerminalClient, data string) error {
	return m.instances.InputPending(id, client, data)
}

func (m *Manager) Resize(id string, client ports.TerminalClient, cols, rows int) error {
	return m.instances.Resize(id, client, cols, rows)
}

func (m *Manager) ResizePending(id string, client ports.TerminalClient, cols, rows int) error {
	return m.instances.ResizePending(id, client, cols, rows)
}

func (m *Manager) ClaimControl(id string, client ports.TerminalClient) error {
	return m.instances.ClaimControl(id, client)
}

func (m *Manager) ClaimPendingControl(id string, client ports.TerminalClient) error {
	return m.instances.ClaimPendingControl(id, client)
}

func (m *Manager) TouchPending(id string) { m.instances.TouchPending(id) }

func (m *Manager) PendingOwner(id string) string { return m.instances.PendingOwner(id) }

func (m *Manager) PromotePending(id string, meta domain.ConnectionInstanceMeta) (Summary, error) {
	return m.instances.PromotePending(id, meta)
}

func (m *Manager) CreatePendingProcessOwned(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, ownerID string, onMarker func(string), onExit func(ports.TerminalExitStatus)) (Summary, error) {
	return m.instances.CreatePendingProcessOwned(ctx, meta, argv, extraEnv, ownerID, onMarker, onExit)
}

func (m *Manager) CreateProcessWithExit(ctx context.Context, meta domain.ConnectionInstanceMeta, argv []string, extraEnv []string, onExit func(ports.TerminalExitStatus)) (Summary, error) {
	return m.instances.CreateProcessWithExit(ctx, meta, argv, extraEnv, onExit)
}

func (m *Manager) MarkSourceState(id, state string) error {
	return m.instances.MarkSourceState(id, state)
}

func (m *Manager) MarkGenerationResult(id, state, detail string) error {
	return m.instances.MarkGenerationResult(id, state, detail)
}

func (m *Manager) SetTitle(id string, title *string) (Summary, error) {
	return m.instances.SetTitle(id, title)
}

func (m *Manager) AbortPending(ctx context.Context, id string) error {
	return m.instances.AbortPending(ctx, id)
}

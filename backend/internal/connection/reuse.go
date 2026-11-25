package connection

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (m *Manager) createReuseOwned(ctx context.Context, definitionID, alias string, resolved definition.RuntimeSSH, cols, rows int, sourceID, ownerID string) (Summary, error) {
	m.transportPool.mu.Lock()
	transport := m.transportPool.instances[sourceID]
	if transport != nil && transport.OwnerID != sourceID {
		transport = m.transportPool.transports[transport.OwnerID]
	}
	m.transportPool.mu.Unlock()
	if transport == nil {
		return Summary{}, ErrTransportUnavailable
	}
	var source Summary
	for _, item := range m.instances.Summaries() {
		if item.ID == sourceID {
			source = item
			break
		}
	}
	if source.ID == "" || source.Type != "ssh" || source.Lifecycle != "live" || source.SourceState != "current" {
		return Summary{}, ErrTransportUnavailable
	}
	if transport.Alias != alias || source.SourceHostAlias == nil || *source.SourceHostAlias != alias {
		return Summary{}, errors.New("reuse definition does not match transport")
	}
	m.transportPool.mu.Lock()
	stale := transport.Draining || transport.SourceRevision != resolved.SourceRevision
	m.transportPool.mu.Unlock()
	if stale {
		m.transportPool.mu.Lock()
		transport.Draining = true
		transport.SourceState = "changed"
		m.transportPool.mu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	if !m.transportReady(transport) {
		return Summary{}, ErrTransportUnavailable
	}
	if resolved.Option.Enabled {
		return m.createReuseTmux(ctx, definitionID, alias, resolved, transport, cols, rows, sourceID, ownerID)
	}
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	meta := newReuseMeta(m, id, definitionID, &alias, cols, rows, sourceID)
	argv := []string{m.sshPath, "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "--", alias}
	m.transportPool.mu.Lock()
	// ControlPersist keeps the mux usable after the original owner exits as
	// long as at least one channel still references it. Only an explicitly
	// draining or channel-less transport is unavailable for another reuse.
	if !transportAcceptsReuse(transport) {
		m.transportPool.mu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	transport.Channels++
	m.transportPool.instances[id] = transport
	m.transportPool.mu.Unlock()
	result, err := m.CreateProcessWithExit(ctx, meta, argv, nil, func(ctx context.Context, _ ports.TerminalExitStatus) {
		m.finishInstance(ctx, id, true)
	})
	if err != nil {
		m.transportPool.mu.Lock()
		transport.Channels--
		delete(m.transportPool.instances, id)
		m.transportPool.mu.Unlock()
		return Summary{}, err
	}
	return result, nil
}

func newReuseMeta(m *Manager, id, definitionID string, alias *string, cols, rows int, sourceID string) domain.ConnectionInstanceMeta {
	now := m.clock.Now().UTC()
	return domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: alias, Lifecycle: "live", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now, AutomaticTitle: *alias, ReuseFromConnectionInstanceID: &sourceID}
}

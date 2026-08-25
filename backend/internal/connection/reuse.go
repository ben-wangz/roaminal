package connection

import (
	"context"
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (m *Manager) createReuse(ctx context.Context, definitionID string, cols, rows int, sourceID string) (Summary, error) {
	return m.createReuseOwned(ctx, definitionID, cols, rows, sourceID, "")
}

func (m *Manager) createReuseOwned(ctx context.Context, definitionID string, cols, rows int, sourceID, ownerID string) (Summary, error) {
	if m.sshPath == "" || m.configRepo == nil {
		return Summary{}, ErrTransportUnavailable
	}
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
	m.transportPool.mu.Lock()
	revision := transport.SourceRevision
	draining, alias := transport.Draining, transport.Alias
	m.transportPool.mu.Unlock()
	currentRevision, fingerprintErr := m.sourceRevision(alias)
	if fingerprintErr != nil {
		return Summary{}, ErrTransportUnavailable
	}
	if draining || revision != currentRevision {
		m.transportPool.mu.Lock()
		transport.Draining = true
		transport.SourceState = "changed"
		m.transportPool.mu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	if !m.transportReady(transport) {
		return Summary{}, ErrTransportUnavailable
	}
	definitionAlias, err := aliasFromDefinitionID(definitionID)
	if err != nil || definitionAlias != alias || source.SourceHostAlias == nil || *source.SourceHostAlias != definitionAlias {
		return Summary{}, errors.New("reuse definition does not match transport")
	}
	if option, ok := m.tmuxOptionForAlias(definitionAlias); ok {
		return m.createReuseTmux(ctx, definitionID, definitionAlias, option, transport, cols, rows, sourceID, ownerID)
	}
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	aliasPtr := definitionAlias
	meta := newReuseMeta(m, id, definitionID, &aliasPtr, cols, rows, sourceID)
	argv := []string{m.sshPath, "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "--", definitionAlias}
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
	result, err := m.CreateProcessWithExit(ctx, meta, argv, nil, func(_ ports.TerminalExitStatus) {
		m.finishInstance(context.Background(), id, true)
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

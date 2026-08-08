package connection

import (
	"context"
	"errors"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

func (m *Manager) createReuse(ctx context.Context, definitionID string, cols, rows int, sourceID string) (Summary, error) {
	if m.sshPath == "" || m.configRepo == nil {
		return Summary{}, ErrTransportUnavailable
	}
	m.transportMu.Lock()
	transport := m.instances[sourceID]
	if transport != nil && transport.OwnerID != sourceID {
		transport = m.transports[transport.OwnerID]
	}
	m.transportMu.Unlock()
	if transport == nil {
		return Summary{}, ErrTransportUnavailable
	}
	var source Summary
	for _, item := range m.Manager.Summaries() {
		if item.ID == sourceID {
			source = item
			break
		}
	}
	if source.ID == "" || source.Type != "ssh" || source.Lifecycle != "live" || source.SourceState != "current" {
		return Summary{}, ErrTransportUnavailable
	}
	collection, err := m.configRepo.Collection(keySet(m.keys))
	if err != nil {
		return Summary{}, err
	}
	m.transportMu.Lock()
	draining, revision, alias := transport.Draining, transport.ContextRevision, transport.Alias
	m.transportMu.Unlock()
	if draining || revision != collection.ETag {
		m.transportMu.Lock()
		transport.Draining = true
		m.transportMu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	if !m.transportReady(transport) {
		return Summary{}, ErrTransportUnavailable
	}
	definitionAlias, err := aliasFromDefinitionID(definitionID)
	if err != nil || definitionAlias != alias || source.SourceHostAlias == nil || *source.SourceHostAlias != definitionAlias {
		return Summary{}, errors.New("reuse definition does not match transport")
	}
	id := terminalID()
	aliasPtr := definitionAlias
	meta := newReuseMeta(m, id, definitionID, &aliasPtr, cols, rows, sourceID)
	argv := []string{m.sshPath, "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "--", definitionAlias}
	m.transportMu.Lock()
	if transport.Draining || transport.OwnerClosed {
		m.transportMu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	transport.Channels++
	m.instances[id] = transport
	m.transportMu.Unlock()
	result, err := m.CreateProcessWithExit(ctx, meta, argv, nil, func(_ terminal.ExitStatus) {
		m.finishInstance(context.Background(), id, true)
	})
	if err != nil {
		m.transportMu.Lock()
		transport.Channels--
		delete(m.instances, id)
		m.transportMu.Unlock()
		return Summary{}, err
	}
	return result, nil
}

func newReuseMeta(m *Manager, id, definitionID string, alias *string, cols, rows int, sourceID string) persistence.SessionMeta {
	return persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: alias, Lifecycle: "live", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AutomaticTitle: *alias, ReuseFromConnectionInstanceID: &sourceID}
}

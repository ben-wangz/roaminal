package connection

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (m *Manager) CreateRemote(ctx context.Context, definitionID string, cols, rows int, reuseFrom string) (Summary, error) {
	result, err := m.createRemoteOwned(ctx, definitionID, cols, rows, reuseFrom, "", false)
	return m.projectSummary(result), err
}

func (m *Manager) createRemoteOwned(ctx context.Context, definitionID string, cols, rows int, reuseFrom, ownerID string, requireTmux bool) (Summary, error) {
	if m.sshPath == "" || m.definitions == nil || m.runtimeDir == "" {
		return Summary{}, ErrTransportUnavailable
	}
	alias, err := aliasFromDefinitionID(definitionID)
	if err != nil {
		return Summary{}, err
	}
	resolved, err := m.definitions.ResolveSSH(alias, m.sourceRevision)
	if err == os.ErrNotExist {
		return Summary{}, errors.New("connection definition not found")
	}
	if err != nil {
		return Summary{}, err
	}
	if requireTmux && !resolved.Option.Enabled {
		return Summary{}, ErrTmuxNotEnabled
	}
	if reuseFrom != "" {
		return m.createReuseOwned(ctx, definitionID, alias, resolved, cols, rows, reuseFrom, ownerID)
	}
	if resolved.Option.Enabled {
		return m.createRemoteTmux(ctx, definitionID, alias, resolved, cols, rows, ownerID)
	}
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	// Unix domain sockets have a small path limit. Keep the persisted session
	// ID intact, but use a short unique transport directory name for the mux.
	transportDir := filepath.Join(m.runtimeDir, "t-"+shortPathToken(id))
	if err := os.MkdirAll(transportDir, 0o700); err != nil {
		return Summary{}, err
	}
	controlPath := filepath.Join(transportDir, "ctl")
	transport := &Transport{Alias: alias, ControlPath: controlPath, SourceRevision: resolved.SourceRevision, SourceState: "current", OwnerID: id, Channels: 1}
	aliasPtr := alias
	now := m.clock.Now().UTC()
	meta := domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "live", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now, AutomaticTitle: alias}
	argv := []string{m.sshPath, "-o", "ControlMaster=yes", "-o", "ControlPersist=yes", "-o", "ControlPath=" + controlPath, "--", alias}
	m.transportPool.mu.Lock()
	m.transportPool.transports[id] = transport
	m.transportPool.instances[id] = transport
	m.transportPool.mu.Unlock()
	result, err := m.CreateProcessWithExit(ctx, meta, argv, nil, func(ctx context.Context, _ ports.TerminalExitStatus) {
		m.finishInstance(ctx, id, true)
	})
	if err != nil {
		m.transportPool.mu.Lock()
		delete(m.transportPool.instances, id)
		delete(m.transportPool.transports, id)
		m.transportPool.mu.Unlock()
		_ = os.RemoveAll(transportDir)
		return Summary{}, err
	}
	return result, nil
}

func (m *Manager) CreateRemoteLaunch(ctx context.Context, definitionID string, cols, rows int, reuseFrom string) (Summary, error) {
	return m.CreateRemoteLaunchOwned(ctx, definitionID, cols, rows, reuseFrom, "")
}

func (m *Manager) CreateRemoteLaunchOwned(ctx context.Context, definitionID string, cols, rows int, reuseFrom, ownerID string) (Summary, error) {
	result, err := m.createRemoteOwned(ctx, definitionID, cols, rows, reuseFrom, ownerID, true)
	return m.projectSummary(result), err
}

func (m *Manager) AbortRemoteLaunch(ctx context.Context, id string) error {
	return m.instances.AbortPending(ctx, id)
}

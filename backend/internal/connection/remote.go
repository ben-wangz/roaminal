package connection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

func (m *Manager) CreateRemote(ctx context.Context, definitionID string, cols, rows int, reuseFrom string) (Summary, error) {
	return m.createRemoteOwned(ctx, definitionID, cols, rows, reuseFrom, "")
}

func (m *Manager) createRemoteOwned(ctx context.Context, definitionID string, cols, rows int, reuseFrom, ownerID string) (Summary, error) {
	if reuseFrom != "" {
		return m.createReuseOwned(ctx, definitionID, cols, rows, reuseFrom, ownerID)
	}
	if m.sshPath == "" || m.configRepo == nil || m.runtimeDir == "" {
		return Summary{}, ErrTransportUnavailable
	}
	alias, err := aliasFromDefinitionID(definitionID)
	if err != nil {
		return Summary{}, err
	}
	collection, err := m.configRepo.Collection(keySet(m.keys))
	if err != nil {
		return Summary{}, err
	}
	var definition *sshconfig.Definition
	for index := range collection.Definitions {
		if collection.Definitions[index].HostAlias == alias {
			definition = &collection.Definitions[index]
			break
		}
	}
	if definition == nil {
		return Summary{}, errors.New("connection definition not found")
	}
	aliases := make(map[string]bool)
	for _, item := range collection.Definitions {
		if item.Type == "ssh" {
			aliases[item.HostAlias] = true
		}
	}
	if options, optionsErr := m.tmuxOptions(aliases); optionsErr == nil {
		if option, ok := options[alias]; ok && option.Enabled {
			return m.createRemoteTmux(ctx, definitionID, alias, definition, collection.ETag, option, cols, rows, ownerID)
		}
	}
	id := terminalID()
	// Unix domain sockets have a small path limit. Keep the persisted session
	// ID intact, but use a short unique transport directory name for the mux.
	transportDir := filepath.Join(m.runtimeDir, "t-"+shortPathToken(id))
	if err := os.MkdirAll(transportDir, 0o700); err != nil {
		return Summary{}, err
	}
	controlPath := filepath.Join(transportDir, "ctl")
	transport := &Transport{Alias: alias, ControlPath: controlPath, ContextRevision: collection.ETag, SourceRevision: collection.ETag, OwnerID: id, Channels: 1}
	aliasPtr := alias
	meta := persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "live", SourceState: "current", HostVerificationAssessment: definition.HostVerificationAssessment, Cols: cols, Rows: rows, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AutomaticTitle: alias}
	argv := []string{m.sshPath, "-o", "ControlMaster=yes", "-o", "ControlPersist=yes", "-o", "ControlPath=" + controlPath, "--", alias}
	m.transportMu.Lock()
	m.transports[id] = transport
	m.instances[id] = transport
	m.transportMu.Unlock()
	result, err := m.CreateProcessWithExit(ctx, meta, argv, nil, func(_ terminal.ExitStatus) {
		m.finishInstance(context.Background(), id, true)
	})
	if err != nil {
		m.transportMu.Lock()
		delete(m.instances, id)
		delete(m.transports, id)
		m.transportMu.Unlock()
		_ = os.RemoveAll(transportDir)
		return Summary{}, err
	}
	if !m.transportReady(transport) {
		transport.Draining = false
	}
	return result, nil
}

func (m *Manager) CreateRemoteLaunch(ctx context.Context, definitionID string, cols, rows int, reuseFrom string) (Summary, error) {
	return m.CreateRemoteLaunchOwned(ctx, definitionID, cols, rows, reuseFrom, "")
}

func (m *Manager) CreateRemoteLaunchOwned(ctx context.Context, definitionID string, cols, rows int, reuseFrom, ownerID string) (Summary, error) {
	alias, err := aliasFromDefinitionID(definitionID)
	if err != nil {
		return Summary{}, err
	}
	if option, ok := m.tmuxOptionForAlias(alias); !ok || !option.Enabled {
		return Summary{}, ErrTmuxNotEnabled
	}
	return m.createRemoteOwned(ctx, definitionID, cols, rows, reuseFrom, ownerID)
}

func (m *Manager) AbortRemoteLaunch(ctx context.Context, id string) error {
	return m.Manager.AbortPending(ctx, id)
}

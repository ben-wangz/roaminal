package connection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

func tmuxLaunchRevision(option connectionoptions.Tmux) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%t\x00%s", option.Enabled, option.SessionName)))
	return hex.EncodeToString(digest[:])
}

// tmuxRemoteCommand intentionally uses only the user's OpenSSH transport and
// the remote tmux binary. The preflight is kept in the same remote command so
// a missing tmux never becomes a published Roaminal connection.
func tmuxRemoteCommand(sessionName, marker string) string {
	script := fmt.Sprintf(`if ! command -v tmux >/dev/null 2>&1; then
  printf 'Roaminal: tmux command not found\n' >&2
  exit 127
fi
tmux ls >/dev/null 2>&1
status=$?
if [ "$status" -gt 1 ]; then
  printf 'Roaminal: tmux session probe failed\n' >&2
  exit "$status"
fi
printf '\033]777;roaminal;tmux-ready:%s\a'
exec tmux new-session -A -s %s`, marker, shellQuote(sessionName))
	return "sh -c " + shellQuote(script)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (m *Manager) createRemoteTmux(ctx context.Context, definitionID, alias string, definition *sshconfig.Definition, sourceRevision string, option connectionoptions.Tmux, cols, rows int, ownerID string) (Summary, error) {
	if !connectionoptions.ValidSessionName(option.SessionName) {
		return Summary{}, connectionoptions.ErrInvalidSessionName
	}
	id := terminalID()
	transportDir := filepath.Join(m.runtimeDir, "t-"+shortPathToken(id))
	if err := os.MkdirAll(transportDir, 0o700); err != nil {
		return Summary{}, err
	}
	controlPath := filepath.Join(transportDir, "ctl")
	transport := &Transport{Alias: alias, ControlPath: controlPath, ContextRevision: sourceRevision, SourceRevision: sourceRevision, TmuxLaunchRevision: tmuxLaunchRevision(option), OwnerID: id, Channels: 1}
	aliasPtr := alias
	meta := persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "pending", SourceState: "current", HostVerificationAssessment: definition.HostVerificationAssessment, Cols: cols, Rows: rows, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AutomaticTitle: alias, TmuxEnabled: true, TmuxSessionName: option.SessionName}
	marker := randomToken()
	argv := []string{m.sshPath, "-tt", "-o", "ControlMaster=yes", "-o", "ControlPersist=yes", "-o", "ControlPath=" + controlPath, "--", alias, tmuxRemoteCommand(option.SessionName, marker)}
	m.transportMu.Lock()
	m.transports[id] = transport
	m.instances[id] = transport
	m.transportMu.Unlock()
	onMarker := func(value string) {
		if value != marker {
			return
		}
		current, ok := m.tmuxOptionForAlias(alias)
		if !ok || current.SessionName != option.SessionName || !current.Enabled {
			_ = m.AbortPending(context.Background(), id)
			return
		}
		if _, err := m.Manager.PromotePending(id, meta); err != nil {
			_ = m.AbortPending(context.Background(), id)
		}
	}
	result, err := m.Manager.CreatePendingProcessOwned(ctx, meta, argv, nil, ownerID, onMarker, func(_ terminal.ExitStatus) {
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
	return result, nil
}

func (m *Manager) createReuseTmux(ctx context.Context, definitionID, alias string, option connectionoptions.Tmux, transport *Transport, cols, rows int, sourceID, ownerID string) (Summary, error) {
	id := terminalID()
	aliasPtr := alias
	meta := persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "pending", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AutomaticTitle: alias, TmuxEnabled: true, TmuxSessionName: option.SessionName, ReuseFromConnectionInstanceID: &sourceID}
	marker := randomToken()
	argv := []string{m.sshPath, "-tt", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "--", alias, tmuxRemoteCommand(option.SessionName, marker)}
	m.transportMu.Lock()
	if !transportAcceptsReuse(transport) {
		m.transportMu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	transport.Channels++
	m.instances[id] = transport
	m.transportMu.Unlock()
	onMarker := func(value string) {
		if value != marker {
			return
		}
		current, ok := m.tmuxOptionForAlias(alias)
		if !ok || current.SessionName != option.SessionName || !current.Enabled {
			_ = m.AbortPending(context.Background(), id)
			return
		}
		if _, err := m.Manager.PromotePending(id, meta); err != nil {
			_ = m.AbortPending(context.Background(), id)
		}
	}
	result, err := m.Manager.CreatePendingProcessOwned(ctx, meta, argv, nil, ownerID, onMarker, func(_ terminal.ExitStatus) {
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

func (m *Manager) tmuxOptionForAlias(alias string) (connectionoptions.Tmux, bool) {
	aliases := map[string]bool{alias: true}
	if m.configRepo != nil {
		if collection, err := m.configRepo.Collection(keySet(m.keys)); err == nil {
			if !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing" {
				return connectionoptions.Tmux{}, false
			}
			aliases = make(map[string]bool)
			for _, definition := range collection.Definitions {
				if definition.Type == "ssh" {
					aliases[definition.HostAlias] = true
				}
			}
		}
	}
	options, err := m.tmuxOptions(aliases)
	if err != nil {
		return connectionoptions.Tmux{}, false
	}
	option, ok := options[alias]
	return option, ok && option.Enabled
}

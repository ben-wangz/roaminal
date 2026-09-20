package connection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
)

func tmuxLaunchRevision(option connectionoptions.Tmux) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%t\x00%s\x00%s", option.Enabled, option.SessionName, option.Pwd)))
	return hex.EncodeToString(digest[:])
}

// tmuxRemoteCommand intentionally uses only the user's OpenSSH transport and
// the remote tmux binary. The preflight is kept in the same remote command so
// a missing tmux never becomes a published Roaminal connection.
func tmuxRemoteCommand(sessionName, pwd, marker string) string {
	return "sh -c " + shellQuote(tmuxRemoteScript(sessionName, pwd, marker))
}

func tmuxRemoteScript(sessionName, pwd, marker string) string {
	return fmt.Sprintf(`if ! command -v tmux >/dev/null 2>&1; then
  printf 'Roaminal: tmux command not found\n' >&2
  exit 127
fi

configured_pwd=%s
case "$configured_pwd" in
  '$HOME')
    start_dir="$HOME"
    ;;
  '$HOME/'*)
    start_dir="$HOME/${configured_pwd#'$HOME/'}"
    ;;
  '~')
    start_dir="$HOME"
    ;;
  '~/'*)
    start_dir="$HOME/${configured_pwd#'~/'}"
    ;;
  /*)
    start_dir="$configured_pwd"
    ;;
  *)
    printf 'Roaminal: invalid tmux start directory\n' >&2
    exit 2
    ;;
esac

tmux has-session -t %s >/dev/null 2>&1
session_status=$?
if [ "$session_status" -gt 1 ]; then
  printf 'Roaminal: tmux session probe failed\n' >&2
  exit "$session_status"
fi
if [ "$session_status" -ne 0 ]; then
  if ! (cd "$start_dir" >/dev/null 2>&1); then
    printf 'Roaminal: tmux start directory is not accessible: %%s\n' "$start_dir" >&2
    exit 1
  fi
fi
printf '\033]777;roaminal;tmux-ready:%s\a'
exec tmux new-session -A -s %s -c "$start_dir"`, shellQuote(pwd), shellQuote(sessionName), marker, shellQuote(sessionName))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (m *Manager) createRemoteTmux(ctx context.Context, definitionID, alias string, definition *sshconfig.Definition, sourceRevision string, option connectionoptions.Tmux, cols, rows int, ownerID string) (Summary, error) {
	if !connectionoptions.ValidSessionName(option.SessionName) {
		return Summary{}, connectionoptions.ErrInvalidSessionName
	}
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	transportDir := filepath.Join(m.runtimeDir, "t-"+shortPathToken(id))
	if err := os.MkdirAll(transportDir, 0o700); err != nil {
		return Summary{}, err
	}
	controlPath := filepath.Join(transportDir, "ctl")
	transport := &Transport{Alias: alias, ControlPath: controlPath, SourceRevision: sourceRevision, SourceState: "current", TmuxLaunchRevision: tmuxLaunchRevision(option), OwnerID: id, Channels: 1}
	aliasPtr := alias
	now := m.clock.Now().UTC()
	meta := domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "pending", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now, AutomaticTitle: alias, TmuxEnabled: true, TmuxSessionName: option.SessionName}
	marker := m.randomToken()
	argv := []string{m.sshPath, "-tt", "-o", "ControlMaster=yes", "-o", "ControlPersist=yes", "-o", "ControlPath=" + controlPath, "--", alias, tmuxRemoteCommand(option.SessionName, option.Pwd, marker)}
	m.transportPool.mu.Lock()
	m.transportPool.transports[id] = transport
	m.transportPool.instances[id] = transport
	m.transportPool.mu.Unlock()
	onMarker := func(value string) {
		if value != marker {
			return
		}
		current, ok := m.tmuxOptionForAlias(alias)
		if !ok || current.SessionName != option.SessionName || !current.Enabled {
			_ = m.AbortPending(context.Background(), id)
			return
		}
		meta.TmuxPrefixKey, meta.TmuxPrefixSource = probeTmuxPrefix(context.Background(), m, transport)
		if _, err := m.instances.PromotePending(id, meta); err != nil {
			_ = m.AbortPending(context.Background(), id)
		}
	}
	result, err := m.instances.CreatePendingProcessOwned(ctx, meta, argv, nil, ownerID, onMarker, func(_ ports.TerminalExitStatus) {
		m.finishInstance(context.Background(), id, true)
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

func (m *Manager) createReuseTmux(ctx context.Context, definitionID, alias string, option connectionoptions.Tmux, transport *Transport, cols, rows int, sourceID, ownerID string) (Summary, error) {
	id, err := m.newID()
	if err != nil {
		return Summary{}, err
	}
	aliasPtr := alias
	now := m.clock.Now().UTC()
	meta := domain.ConnectionInstanceMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "pending", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: now, UpdatedAt: now, AutomaticTitle: alias, TmuxEnabled: true, TmuxSessionName: option.SessionName, ReuseFromConnectionInstanceID: &sourceID}
	marker := m.randomToken()
	argv := []string{m.sshPath, "-tt", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "--", alias, tmuxRemoteCommand(option.SessionName, option.Pwd, marker)}
	m.transportPool.mu.Lock()
	if !transportAcceptsReuse(transport) {
		m.transportPool.mu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	transport.Channels++
	m.transportPool.instances[id] = transport
	m.transportPool.mu.Unlock()
	onMarker := func(value string) {
		if value != marker {
			return
		}
		current, ok := m.tmuxOptionForAlias(alias)
		if !ok || current.SessionName != option.SessionName || !current.Enabled {
			_ = m.AbortPending(context.Background(), id)
			return
		}
		meta.TmuxPrefixKey, meta.TmuxPrefixSource = probeTmuxPrefix(context.Background(), m, transport)
		if _, err := m.instances.PromotePending(id, meta); err != nil {
			_ = m.AbortPending(context.Background(), id)
		}
	}
	result, err := m.instances.CreatePendingProcessOwned(ctx, meta, argv, nil, ownerID, onMarker, func(_ ports.TerminalExitStatus) {
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

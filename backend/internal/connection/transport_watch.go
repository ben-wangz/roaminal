package connection

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/processenv"
)

func (m *Manager) watchSources(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refreshSources()
		}
	}
}

func (m *Manager) refreshSources() {
	if m.definitions == nil {
		return
	}
	m.transportPool.mu.Lock()
	transports := make([]*Transport, 0, len(m.transportPool.transports))
	aliases := map[string]bool{}
	for _, transport := range m.transportPool.transports {
		transports = append(transports, transport)
		aliases[transport.Alias] = true
	}
	m.transportPool.mu.Unlock()
	if len(transports) == 0 {
		return
	}
	collection, err := m.definitions.NativeCollection()
	if err != nil {
		return
	}
	current := map[string]bool{}
	revisions := map[string]string{}
	unresolved := map[string]bool{}
	for _, definition := range collection.Definitions {
		alias := definition.HostAlias
		if definition.Type != "ssh" || !aliases[alias] || current[alias] {
			continue
		}
		current[alias] = true
		if revision, err := m.sourceRevision(alias); err == nil {
			revisions[alias] = revision
		} else {
			// An inconclusive probe is not evidence of a changed source.
			unresolved[alias] = true
		}
	}
	// Do not hold the definition lock while probing every live alias. Discard
	// a pass that overlapped a native edit or cannot confirm a committed view.
	if latest, err := m.definitions.NativeCollection(); err != nil || latest.ETag != collection.ETag {
		return
	}
	configUnavailable := !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing"
	for _, warning := range collection.ConfigSource.Warnings {
		if warning.Class == "include_ignored" {
			// Aliases supplied by an Include are not visible here.
			configUnavailable = true
			break
		}
	}
	for _, transport := range transports {
		state := transportSourceState(transport, revisions, unresolved, configUnavailable, current)
		if state == "" {
			continue
		}
		m.transportPool.mu.Lock()
		if m.transportPool.transports[transport.OwnerID] != transport {
			m.transportPool.mu.Unlock()
			continue
		}
		// Drain only this transport and the instances still attached to it.
		// A newer connection with the same alias may already be current.
		transport.Draining = true
		transport.SourceState = state
		var instanceIDs []string
		for id, attached := range m.transportPool.instances {
			if attached == transport {
				instanceIDs = append(instanceIDs, id)
			}
		}
		m.transportPool.mu.Unlock()
		for _, id := range instanceIDs {
			_ = m.instances.MarkSourceState(id, state)
		}
	}
}

func transportSourceState(transport *Transport, revisions map[string]string, unresolved map[string]bool, configUnavailable bool, current map[string]bool) string {
	if !configUnavailable && !current[transport.Alias] {
		return "deleted"
	}
	if unresolved[transport.Alias] {
		return ""
	}
	if revision, ok := revisions[transport.Alias]; ok && transport.SourceRevision != revision {
		return "changed"
	}
	return ""
}

func (m *Manager) transportReady(transport *Transport) bool {
	if !m.validControlPath(transport.ControlPath) {
		return false
	}
	if m.sshPath == "" {
		return false
	}
	cmd := exec.Command(m.sshPath, "-F", "none", "-S", transport.ControlPath, "-O", "check", "--", transport.Alias)
	cmd.Env = processenv.Environment()
	return cmd.Run() == nil
}

func (m *Manager) validControlPath(path string) bool {
	if m.runtimeDir == "" || !ownedPrivateDir(m.runtimeDir) {
		return false
	}
	rel, err := filepath.Rel(m.runtimeDir, filepath.Dir(path))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) || !ownedPrivateDir(filepath.Dir(path)) {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return false
	}
	return ownedByCurrentUID(info)
}

func (m *Manager) prepareRuntimeDir(id string) (string, error) {
	if strings.ContainsAny(id, `/\\`) {
		id = ""
	}
	if id == "" {
		id = m.randomToken()
	}
	root := "/tmp/roaminal-mux"
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(root, 0o700); err != nil {
			return "", err
		}
	} else if err != nil || !ownedPrivateDir(root) {
		return "", errors.New("unsafe ssh mux root")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", err
	}
	name := "rm-" + shortPathToken(id)
	current := filepath.Join(root, name)
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.Name() == name || !strings.HasPrefix(entry.Name(), "rm-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if ownedPrivateDir(path) {
			_ = os.RemoveAll(path)
		}
	}
	if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if !ownedPrivateDir(current) {
		return "", errors.New("unsafe ssh runtime directory")
	}
	return current, nil
}

// shortPathToken keeps temporary mux paths below the Unix socket path limit.
func shortPathToken(id string) string {
	token := strings.NewReplacer("-", "", "/", "", "\\", "").Replace(id)
	if token == "" {
		token = "runtime"
	}
	if len(token) > 12 {
		token = token[:12]
	}
	return token
}

func ownedPrivateDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() && info.Mode().Perm()&0o077 == 0 && ownedByCurrentUID(info)
}

func ownedByCurrentUID(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint32(stat.Uid) == uint32(os.Getuid())
}
func discover(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}
func (m *Manager) randomToken() string {
	var raw [8]byte
	if m.random != nil {
		if _, err := m.random.Read(raw[:]); err == nil {
			return fmt.Sprintf("%x", raw[:])
		}
	}
	return fmt.Sprintf("%x", m.clock.Now().UnixNano())
}

func (m *Manager) newID() (string, error) {
	if m.ids == nil {
		return "", ports.ErrIDUnavailable
	}
	return m.ids.NewID()
}
func aliasFromDefinitionID(id string) (string, error) {
	if !strings.HasPrefix(id, "ssh.") {
		return "", errors.New("invalid ssh definition id")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "ssh."))
	if err != nil || len(data) == 0 {
		return "", errors.New("invalid ssh definition id")
	}
	return string(data), nil
}

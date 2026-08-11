package connection

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
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
	if m.configRepo == nil {
		return
	}
	collection, err := m.configRepo.Collection(keySet(m.keys))
	if err != nil {
		return
	}
	current := map[string]bool{}
	for _, definition := range collection.Definitions {
		if definition.Type == "ssh" {
			current[definition.HostAlias] = true
		}
	}
	configUnavailable := !collection.ConfigSource.Readable && collection.ConfigSource.Status != "missing"
	m.transportMu.Lock()
	transports := make([]*Transport, 0, len(m.transports))
	for _, transport := range m.transports {
		transports = append(transports, transport)
		shouldDrain := false
		revision := transport.SourceRevision
		if revision != collection.ETag {
			if !transport.Draining {
				transport.Draining = true
				shouldDrain = true
			}
		}
		transport.stopRequested = transport.stopRequested || shouldDrain
	}
	m.transportMu.Unlock()
	for _, transport := range transports {
		m.transportMu.Lock()
		draining := transport.Draining
		stopRequested := transport.stopRequested
		m.transportMu.Unlock()
		if draining && stopRequested {
			m.drainTransport(transport)
			m.transportMu.Lock()
			transport.stopRequested = false
			m.transportMu.Unlock()
		}
		state := transportSourceState(transport, collection.ETag, configUnavailable, current)
		if state == "" {
			continue
		}
		for _, summary := range m.Summaries() {
			if summary.SourceHostAlias != nil && *summary.SourceHostAlias == transport.Alias {
				_ = m.Manager.MarkSourceState(summary.ID, state)
			}
		}
	}
}

func transportSourceState(transport *Transport, revision string, configUnavailable bool, current map[string]bool) string {
	if !configUnavailable && !current[transport.Alias] {
		return "deleted"
	}
	transportRevision := transport.SourceRevision
	if transportRevision != revision {
		return "changed"
	}
	return ""
}

func (m *Manager) drainTransport(transport *Transport) {
	if m.sshPath == "" {
		return
	}
	_ = exec.Command(m.sshPath, "-F", "none", "-S", transport.ControlPath, "-O", "stop", "--", transport.Alias).Run()
}

func (m *Manager) stopTransport(_ context.Context, transport *Transport) {
	if m.sshPath != "" {
		_ = exec.Command(m.sshPath, "-F", "none", "-S", transport.ControlPath, "-O", "exit", "--", transport.Alias).Run()
	}
	_ = os.RemoveAll(filepath.Dir(transport.ControlPath))
}
func (m *Manager) transportReady(transport *Transport) bool {
	if !m.validControlPath(transport.ControlPath) {
		return false
	}
	if m.sshPath == "" {
		return false
	}
	return exec.Command(m.sshPath, "-F", "none", "-S", transport.ControlPath, "-O", "check", "--", transport.Alias).Run() == nil
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

func prepareRuntimeDir(id string) (string, error) {
	if strings.ContainsAny(id, `/\\`) {
		id = ""
	}
	if id == "" {
		id = randomToken()
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

// shortPathToken keeps temporary mux paths well below the Unix socket path
// limit while retaining enough entropy to avoid collisions between sessions.
func shortPathToken(id string) string {
	token := strings.NewReplacer("-", "", "/", "", "\\", "").Replace(id)
	if token == "" {
		token = randomToken()
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
func randomToken() string {
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	return fmt.Sprintf("%x", raw[:])
}
func terminalID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
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
func keySet(keys *sshkey.Inventory) map[string]bool {
	if keys == nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	for _, key := range keys.List() {
		result[key.FileName] = true
	}
	return result
}

package connection

import (
	"context"
	"errors"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type EffectiveEndpoint = ports.EffectiveEndpoint

const displayEndpointCacheTTL = 10 * time.Second
const displayEndpointProbeTimeout = 750 * time.Millisecond

type endpointCacheEntry struct {
	endpoint  *EffectiveEndpoint
	expiresAt time.Time
}

// ResolveEndpoint asks the same OpenSSH installation used by the live
// transport for its effective identity. It deliberately does not connect.
func (m *Manager) ResolveEndpoint(ctx context.Context, id string) (EffectiveEndpoint, error) {
	info, err := m.RemoteTransferInfo(id)
	if err != nil {
		return EffectiveEndpoint{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return m.resolveEndpointAlias(ctx, info.SSHPath, info.Alias)
}

func (m *Manager) resolveEndpointAlias(ctx context.Context, sshPath, alias string) (EffectiveEndpoint, error) {
	if strings.TrimSpace(sshPath) == "" || strings.TrimSpace(alias) == "" {
		return EffectiveEndpoint{}, ports.ErrTransportUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, sshPath, "-o", "CanonicalizeHostname=no", "-G", "--", alias).Output()
	if err != nil {
		return EffectiveEndpoint{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && (parts[0] == "user" || parts[0] == "hostname" || parts[0] == "port") {
			values[parts[0]] = parts[1]
		}
	}
	port := 22
	if value := values["port"]; value != "" {
		port, err = strconv.Atoi(value)
		if err != nil {
			return EffectiveEndpoint{}, err
		}
	}
	if strings.TrimSpace(values["user"]) == "" || strings.TrimSpace(values["hostname"]) == "" || port < 1 || port > 65535 {
		return EffectiveEndpoint{}, errors.New("incomplete effective SSH endpoint")
	}
	return EffectiveEndpoint{User: values["user"], Host: values["hostname"], Port: port}, nil
}

func (m *Manager) displayEndpoint(alias *string, connectionType string) *EffectiveEndpoint {
	if connectionType != "ssh" || alias == nil || strings.TrimSpace(*alias) == "" || strings.TrimSpace(m.sshPath) == "" {
		return nil
	}
	key := strings.TrimSpace(*alias)
	now := time.Now()
	if m.clock != nil {
		now = m.clock.Now()
	}
	m.endpointMu.Lock()
	if entry, ok := m.endpointCache[key]; ok && now.Before(entry.expiresAt) {
		endpoint := cloneEndpoint(entry.endpoint)
		m.endpointMu.Unlock()
		return endpoint
	}
	m.endpointMu.Unlock()

	probeContext, cancel := context.WithTimeout(context.Background(), displayEndpointProbeTimeout)
	defer cancel()
	endpoint, err := m.resolveEndpointAlias(probeContext, m.sshPath, key)
	var cached *EffectiveEndpoint
	if err == nil {
		cached = &endpoint
	}
	m.endpointMu.Lock()
	if m.endpointCache == nil {
		m.endpointCache = make(map[string]endpointCacheEntry)
	}
	m.endpointCache[key] = endpointCacheEntry{endpoint: cloneEndpoint(cached), expiresAt: now.Add(displayEndpointCacheTTL)}
	m.endpointMu.Unlock()
	return cached
}

func cloneEndpoint(endpoint *EffectiveEndpoint) *EffectiveEndpoint {
	if endpoint == nil {
		return nil
	}
	copy := *endpoint
	return &copy
}

package connection

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type EffectiveEndpoint struct {
	User string
	Host string
	Port int
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
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, info.SSHPath, "-o", "CanonicalizeHostname=no", "-G", "--", info.Alias).Output()
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

package connection

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

const auxiliaryOutputLimit = 8 * 1024

type cappedBuffer struct {
	data     bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	if b.data.Len()+len(value) > b.limit {
		remaining := b.limit - b.data.Len()
		if remaining > 0 {
			_, _ = b.data.Write(value[:remaining])
		}
		b.overflow = true
		return len(value), nil
	}
	return b.data.Write(value)
}

func (m *Manager) reserveAuxiliary(transport *Transport) bool {
	m.transportMu.Lock()
	defer m.transportMu.Unlock()
	if !transportAcceptsReuse(transport) {
		return false
	}
	transport.AuxiliaryChannels++
	return true
}

func (m *Manager) releaseAuxiliary(transport *Transport) {
	m.transportMu.Lock()
	if transport.AuxiliaryChannels > 0 {
		transport.AuxiliaryChannels--
	}
	shouldStop := transport.OwnerClosed && transport.Channels == 0 && transport.AuxiliaryChannels == 0
	if shouldStop {
		delete(m.transports, transport.OwnerID)
	}
	m.transportMu.Unlock()
	if shouldStop {
		m.clearRemoteState(transport.OwnerID)
		m.stopTransport(context.Background(), transport)
	}
}

func (m *Manager) runAuxiliary(ctx context.Context, transport *Transport, remoteArgs ...string) ([]byte, error) {
	return m.runAuxiliaryInput(ctx, transport, nil, remoteArgs...)
}

func (m *Manager) runAuxiliaryInput(ctx context.Context, transport *Transport, input io.Reader, remoteArgs ...string) ([]byte, error) {
	if m.sshPath == "" || transport == nil || !m.reserveAuxiliary(transport) {
		return nil, ErrTransportUnavailable
	}
	defer m.releaseAuxiliary(transport)
	args := []string{"-T", "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "-o", "BatchMode=yes", "-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none", "--", transport.Alias}
	args = append(args, remoteArgs...)
	command := exec.CommandContext(ctx, m.sshPath, args...)
	stdout, stderr := &cappedBuffer{limit: auxiliaryOutputLimit}, &cappedBuffer{limit: auxiliaryOutputLimit}
	command.Stdout, command.Stderr = stdout, stderr
	command.Stdin = input
	if err := command.Run(); err != nil {
		return nil, err
	}
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("auxiliary output exceeded limit")
	}
	return stdout.data.Bytes(), nil
}

func withAuxiliaryTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, 2*time.Second)
}

var _ io.Writer = (*cappedBuffer)(nil)

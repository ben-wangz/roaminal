package connection

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestRemoteCommandInvocationPreservesPayloadScriptArguments(t *testing.T) {
	command := RemoteCommand{Script: `printf '%s|%s' "$1" "$2"`, Args: []string{"a path", "$HOME"}, Stdin: strings.NewReader("payload")}
	invocation := strings.Join(remoteCommandInvocation(command), " ")
	process := exec.CommandContext(context.Background(), "sh", "-c", invocation)
	output, err := process.Output()
	if err != nil {
		t.Fatalf("run quoted invocation: %v", err)
	}
	if string(output) != "a path|$HOME" {
		t.Fatalf("quoted arguments changed: %q", output)
	}
}

func TestRemoteCommandInvocationQuotesScriptAndArguments(t *testing.T) {
	invocation := remoteCommandInvocation(RemoteCommand{Script: "printf ok", Args: []string{"a'b"}, Stdin: strings.NewReader("payload")})
	if len(invocation) != 5 || invocation[2] != "'printf ok'" || invocation[4] != "'a'\"'\"'b'" {
		t.Fatalf("unexpected remote invocation: %#v", invocation)
	}
}

func TestAuxiliaryLeaseKeepsTransportUntilRelease(t *testing.T) {
	manager := &Manager{transportPool: newTransportPool()}
	transport := &Transport{OwnerID: "owner", Channels: 1, ControlPath: filepath.Join(t.TempDir(), "ctl")}
	manager.transportPool.transports[transport.OwnerID] = transport
	manager.transportPool.instances[transport.OwnerID] = transport

	if !manager.reserveAuxiliary(transport) {
		t.Fatal("registered transport should accept an auxiliary lease")
	}
	manager.finishInstance(context.Background(), transport.OwnerID, true)
	manager.transportPool.mu.Lock()
	_, stillRegistered := manager.transportPool.transports[transport.OwnerID]
	manager.transportPool.mu.Unlock()
	if !stillRegistered {
		t.Fatal("owner exit must keep the transport while an auxiliary lease is active")
	}

	manager.releaseAuxiliary(transport)
	manager.transportPool.mu.Lock()
	_, stillRegistered = manager.transportPool.transports[transport.OwnerID]
	manager.transportPool.mu.Unlock()
	if stillRegistered {
		t.Fatal("transport should be removed after the final auxiliary lease is released")
	}
}

func TestReserveAuxiliaryRejectsUnregisteredTransport(t *testing.T) {
	manager := &Manager{transportPool: newTransportPool()}
	registered := &Transport{OwnerID: "owner", Channels: 1}
	stale := &Transport{OwnerID: "owner", Channels: 1}
	manager.transportPool.transports[registered.OwnerID] = registered
	if manager.reserveAuxiliary(stale) {
		t.Fatal("stale transport pointer must not acquire an auxiliary lease")
	}
}

func TestClassifyAuxiliaryErrorRecognizesControlSocketFailure(t *testing.T) {
	err := classifyAuxiliaryError(errors.New("exit status 255"), []byte("Control socket connect(/tmp/roaminal-mux/ctl): No such file or directory"))
	if !errors.Is(err, ports.ErrTransportUnavailable) {
		t.Fatalf("classified error = %v, want transport unavailable", err)
	}
	remoteErr := classifyAuxiliaryError(errors.New("exit status 1"), []byte("remote command failed"))
	if errors.Is(remoteErr, ports.ErrTransportUnavailable) {
		t.Fatalf("ordinary remote command failure was classified as transport failure: %v", remoteErr)
	}
}

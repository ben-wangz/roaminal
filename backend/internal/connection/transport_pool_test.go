package connection

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type transportTestRuntime struct {
	items []ports.TerminalInstanceSummary
	fatal chan error
}

func newTransportTestRuntime(items ...ports.TerminalInstanceSummary) *transportTestRuntime {
	return &transportTestRuntime{items: items, fatal: make(chan error)}
}

func (r *transportTestRuntime) Start(context.Context) error { return nil }
func (r *transportTestRuntime) Shutdown(context.Context)    {}
func (r *transportTestRuntime) Delete(context.Context, string) error {
	return nil
}
func (r *transportTestRuntime) Fatal() <-chan error { return r.fatal }
func (r *transportTestRuntime) WorkerFatal(error)   {}
func (r *transportTestRuntime) PersistenceDegraded() bool {
	return false
}
func (r *transportTestRuntime) InitialCwd() string { return "/" }
func (r *transportTestRuntime) RuntimeID() string  { return "runtime" }
func (r *transportTestRuntime) Summaries() []ports.TerminalInstanceSummary {
	return append([]ports.TerminalInstanceSummary(nil), r.items...)
}
func (r *transportTestRuntime) Create(context.Context, string, int, int) (ports.TerminalInstanceSummary, error) {
	return ports.TerminalInstanceSummary{}, errors.New("not implemented")
}
func (r *transportTestRuntime) AttachReserved(context.Context, string) (ports.TerminalClient, error) {
	return nil, errors.New("not implemented")
}
func (r *transportTestRuntime) AttachPendingReserved(context.Context, string) (ports.TerminalClient, error) {
	return nil, errors.New("not implemented")
}
func (r *transportTestRuntime) ReserveAttach(string) error        { return nil }
func (r *transportTestRuntime) ReservePendingAttach(string) error { return nil }
func (r *transportTestRuntime) ReleaseAttach(string)              {}
func (r *transportTestRuntime) ReleasePendingAttach(string)       {}
func (r *transportTestRuntime) Detach(string, ports.TerminalClient) {
}
func (r *transportTestRuntime) DetachPending(string, ports.TerminalClient) {
}
func (r *transportTestRuntime) Input(string, ports.TerminalClient, string) error {
	return nil
}
func (r *transportTestRuntime) InputPending(string, ports.TerminalClient, string) error {
	return nil
}
func (r *transportTestRuntime) Resize(string, ports.TerminalClient, int, int) error {
	return nil
}
func (r *transportTestRuntime) ResizePending(string, ports.TerminalClient, int, int) error {
	return nil
}
func (r *transportTestRuntime) ClaimControl(string, ports.TerminalClient) error {
	return nil
}
func (r *transportTestRuntime) ClaimPendingControl(string, ports.TerminalClient) error {
	return nil
}
func (r *transportTestRuntime) TouchPending(string)        {}
func (r *transportTestRuntime) PendingOwner(string) string { return "" }
func (r *transportTestRuntime) PromotePending(string, domain.ConnectionInstanceMeta) (ports.TerminalInstanceSummary, error) {
	return ports.TerminalInstanceSummary{}, errors.New("not implemented")
}
func (r *transportTestRuntime) CreatePendingProcessOwned(context.Context, domain.ConnectionInstanceMeta, []string, []string, string, func(string), func(ports.TerminalExitStatus)) (ports.TerminalInstanceSummary, error) {
	return ports.TerminalInstanceSummary{}, errors.New("not implemented")
}
func (r *transportTestRuntime) CreateProcessWithExit(context.Context, domain.ConnectionInstanceMeta, []string, []string, func(ports.TerminalExitStatus)) (ports.TerminalInstanceSummary, error) {
	return ports.TerminalInstanceSummary{}, errors.New("not implemented")
}
func (r *transportTestRuntime) MarkSourceState(string, string) error { return nil }
func (r *transportTestRuntime) MarkGenerationResult(string, string, string) error {
	return nil
}
func (r *transportTestRuntime) SetTitle(string, *string) (ports.TerminalInstanceSummary, error) {
	return ports.TerminalInstanceSummary{}, errors.New("not implemented")
}
func (r *transportTestRuntime) AbortPending(context.Context, string) error { return nil }

func newReadyTransportManager(t *testing.T, items ...ports.TerminalInstanceSummary) (*Manager, *Transport, func()) {
	t.Helper()
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transportDir := filepath.Join(runtimeDir, "transport")
	if err := os.Mkdir(transportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	controlPath := filepath.Join(transportDir, "ctl")
	listener, err := net.Listen("unix", controlPath)
	if err != nil {
		t.Fatal(err)
	}
	sshPath := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(sshPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	manager := &Manager{
		instances:     NewInstanceService(newTransportTestRuntime(items...)),
		sshPath:       sshPath,
		runtimeDir:    runtimeDir,
		transportPool: newTransportPool(),
	}
	transport := &Transport{OwnerID: "owner", Alias: "alpha", ControlPath: controlPath, Channels: 1, SourceState: "current"}
	manager.transportPool.transports[transport.OwnerID] = transport
	for _, item := range items {
		manager.transportPool.instances[item.ID] = transport
	}
	return manager, transport, func() { _ = listener.Close() }
}

func TestDerivedInstanceMappingSurvivesOwnerExit(t *testing.T) {
	alias := "alpha"
	items := []ports.TerminalInstanceSummary{
		{ID: "owner", ConnectionInstanceID: "owner", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &alias},
		{ID: "derived", ConnectionInstanceID: "derived", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &alias},
	}
	manager, transport, closeFixture := newReadyTransportManager(t, items...)
	defer closeFixture()
	transport.Channels = 2
	manager.transportPool.instances["owner"] = transport
	manager.transportPool.instances["derived"] = transport

	manager.finishInstance(context.Background(), "owner", true)
	manager.transportPool.mu.Lock()
	registered := manager.transportPool.transports[transport.OwnerID] == transport
	derivedMapping := manager.transportPool.instances["derived"] == transport
	ownerClosed, channels := transport.OwnerClosed, transport.Channels
	manager.transportPool.mu.Unlock()
	if !registered || !derivedMapping || !ownerClosed || channels != 1 {
		t.Fatalf("owner exit broke derived mapping: registered=%t derived=%t ownerClosed=%t channels=%d", registered, derivedMapping, ownerClosed, channels)
	}
	if resolved, _, err := manager.lookupRemoteTransport("derived"); err != nil || resolved != transport {
		t.Fatalf("derived lookup after owner exit = %v, %v", resolved, err)
	}

	manager.finishInstance(context.Background(), "derived", true)
	manager.transportPool.mu.Lock()
	_, registered = manager.transportPool.transports[transport.OwnerID]
	_, derivedMapping = manager.transportPool.instances["derived"]
	manager.transportPool.mu.Unlock()
	if registered || derivedMapping {
		t.Fatal("final derived exit did not remove the transport mapping")
	}
}

func TestConcurrentAuxiliaryLeasesKeepLiveMapping(t *testing.T) {
	alias := "alpha"
	items := []ports.TerminalInstanceSummary{{ID: "owner", ConnectionInstanceID: "owner", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &alias}}
	manager, transport, closeFixture := newReadyTransportManager(t, items...)
	defer closeFixture()
	const workers = 24
	var wait sync.WaitGroup
	failures := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if !manager.acquireAuxiliary(context.Background(), transport) {
				failures <- errors.New("auxiliary lease was rejected")
				return
			}
			manager.releaseAuxiliary(transport)
		}()
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	manager.transportPool.mu.Lock()
	auxiliaryChannels := transport.AuxiliaryChannels
	registered := manager.transportPool.transports[transport.OwnerID] == transport
	manager.transportPool.mu.Unlock()
	if auxiliaryChannels != 0 || !registered {
		t.Fatalf("concurrent leases corrupted mapping: auxiliary=%d registered=%t", auxiliaryChannels, registered)
	}
}

func TestRemoteCapabilityReportsHealthyStaleAndDeadStates(t *testing.T) {
	alias := "alpha"
	item := ports.TerminalInstanceSummary{ID: "owner", ConnectionInstanceID: "owner", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &alias}
	manager, transport, closeFixture := newReadyTransportManager(t, item)
	defer closeFixture()
	transport.Draining = true
	transport.SourceState = "changed"
	capability := manager.Summaries()[0].RemoteCapability
	if capability.Status != "source_stale" || capability.Retryable {
		t.Fatalf("healthy stale capability = %+v", capability)
	}

	deadSSH := filepath.Join(t.TempDir(), "ssh-dead")
	if err := os.WriteFile(deadSSH, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager.sshPath = deadSSH
	capability = manager.Summaries()[0].RemoteCapability
	if capability.Status != "transport_unavailable" || !capability.Retryable {
		t.Fatalf("dead stale capability = %+v", capability)
	}
}

func TestDeadControlMasterReturnsRetryableWithoutFallback(t *testing.T) {
	alias := "alpha"
	item := ports.TerminalInstanceSummary{ID: "owner", ConnectionInstanceID: "owner", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &alias}
	manager, transport, closeFixture := newReadyTransportManager(t, item)
	defer closeFixture()
	deadSSH := filepath.Join(t.TempDir(), "ssh-dead")
	if err := os.WriteFile(deadSSH, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager.sshPath = deadSSH
	_, err := manager.AcquireRemoteTransfer(context.Background(), "owner")
	if !errors.Is(err, ports.ErrTransportUnavailable) {
		t.Fatalf("dead control master error = %v, want retryable transport error", err)
	}
	manager.transportPool.mu.Lock()
	auxiliaryChannels := transport.AuxiliaryChannels
	registered := manager.transportPool.transports[transport.OwnerID] == transport
	manager.transportPool.mu.Unlock()
	if auxiliaryChannels != 0 || !registered {
		t.Fatalf("dead control master changed mapping: auxiliary=%d registered=%t", auxiliaryChannels, registered)
	}
}

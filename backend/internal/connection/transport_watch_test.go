package connection

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func TestShortPathTokenKeepsMuxSocketPathBounded(t *testing.T) {
	bootID := "552c38b7-80a0-42d6-995a-5c0267d7461a"
	sessionID := "b2c6e673-c603-4e80-9d68-878525019bd8"
	controlPath := filepath.Join("/tmp/roaminal-mux", "rm-"+shortPathToken(bootID), "t-"+shortPathToken(sessionID), "ctl")
	if got := len(controlPath); got > 70 {
		t.Fatalf("control path length = %d, want <= 70: %s", got, controlPath)
	}
	if got := shortPathToken(strings.Repeat("a", 64)); len(got) != 12 {
		t.Fatalf("short token length = %d, want 12", len(got))
	}
}

func TestTransportSourceStateStaysCurrentWhenConfigIsUnchanged(t *testing.T) {
	transport := &Transport{Alias: "codespace", SourceRevision: "etag-1"}
	current := map[string]bool{"codespace": true}
	revisions := map[string]string{"codespace": "etag-1"}
	unresolved := map[string]bool{}

	if got := transportSourceState(transport, revisions, unresolved, false, current); got != "" {
		t.Fatalf("unchanged transport state = %q, want empty", got)
	}
	if got := transportSourceState(transport, map[string]string{"codespace": "etag-2"}, unresolved, false, current); got != "changed" {
		t.Fatalf("changed config state = %q, want changed", got)
	}
	if got := transportSourceState(transport, revisions, unresolved, false, map[string]bool{}); got != "deleted" {
		t.Fatalf("missing host state = %q, want deleted", got)
	}
	if got := transportSourceState(transport, revisions, unresolved, true, map[string]bool{}); got != "" {
		t.Fatalf("unavailable config state = %q, want empty", got)
	}
	if got := transportSourceState(transport, nil, map[string]bool{"codespace": true}, false, current); got != "" {
		t.Fatalf("inconclusive alias probe state = %q, want empty", got)
	}
}

func TestSourceStaleTransportStillAcceptsAuxiliaryChannels(t *testing.T) {
	transport := &Transport{Channels: 1, Draining: true}
	if transportAcceptsReuse(transport) {
		t.Fatal("source-stale transport must reject new reuse")
	}
	if !transportAcceptsAuxiliary(transport) {
		t.Fatal("source-stale transport must keep existing auxiliary access")
	}
}

func TestClosedOwnerKeepsTransportReusableWhileChannelsRemain(t *testing.T) {
	transport := &Transport{Channels: 1, OwnerClosed: true}
	if !transportAcceptsReuse(transport) {
		t.Fatal("closed owner should not block reuse while a channel remains")
	}

	transport.Draining = true
	if transportAcceptsReuse(transport) {
		t.Fatal("draining transport should block reuse")
	}
	transport.Draining = false
	transport.Channels = 0
	if transportAcceptsReuse(transport) {
		t.Fatal("channel-less transport should block reuse")
	}
}

func TestRefreshSourcesIsolatesUnrelatedChangesAndPreservesHistoricalAccess(t *testing.T) {
	realSSH, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh is unavailable")
	}
	sshDir := t.TempDir()
	if err := os.Chmod(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(sshDir, "config")
	config := "Host *\n  User wildcard\n  Port 2200\nHost alpha\n  HostName alpha.example\nHost beta\n  HostName beta.example\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := sshfs.OpenAt(sshDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	sshPath := filepath.Join(t.TempDir(), "ssh-wrapper")
	script := fmt.Sprintf("#!/bin/sh\nexec %s -F %s \"$@\"\n", shellQuote(realSSH), shellQuote(configPath))
	if err := os.WriteFile(sshPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	repo := sshconfig.New(root)
	aliasAlpha, aliasBeta := "alpha", "beta"
	runtime := newTransportTestRuntime(
		ports.TerminalInstanceSummary{ID: "a-instance", ConnectionInstanceID: "a-instance", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &aliasAlpha},
		ports.TerminalInstanceSummary{ID: "b-instance", ConnectionInstanceID: "b-instance", Type: "ssh", Purpose: "interactive", Lifecycle: "live", SourceState: "current", SourceHostAlias: &aliasBeta},
	)
	manager := &Manager{configRepo: repo, sshPath: sshPath, instances: NewInstanceService(runtime), transportPool: newTransportPool()}
	alpha := &Transport{OwnerID: "a-instance", Alias: "alpha", SourceRevision: "", Channels: 1, SourceState: "current"}
	beta := &Transport{OwnerID: "b-instance", Alias: "beta", SourceRevision: "", Channels: 1, SourceState: "current"}
	alpha.SourceRevision, err = manager.sourceRevision(alpha.Alias)
	if err != nil {
		t.Fatal(err)
	}
	beta.SourceRevision, err = manager.sourceRevision(beta.Alias)
	if err != nil {
		t.Fatal(err)
	}
	manager.transportPool.transports[alpha.OwnerID] = alpha
	manager.transportPool.transports[beta.OwnerID] = beta
	manager.transportPool.instances[alpha.OwnerID] = alpha
	manager.transportPool.instances[beta.OwnerID] = beta

	if err := os.WriteFile(configPath, []byte(config+"Host unrelated\n  HostName unrelated.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.refreshSources()
	if alpha.Draining || beta.Draining {
		t.Fatalf("unrelated Host change drained a transport: alpha=%+v beta=%+v", alpha, beta)
	}

	changedBeta := "Host *\n  User wildcard\n  Port 2200\nHost alpha\n  HostName alpha.example\nHost beta\n  HostName beta-new.example\nHost unrelated\n  HostName unrelated.example\n"
	if err := os.WriteFile(configPath, []byte(changedBeta), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.refreshSources()
	if alpha.Draining {
		t.Fatal("changing beta drained alpha")
	}
	if !beta.Draining || beta.SourceState != "changed" {
		t.Fatalf("changing beta did not drain only beta: %+v", beta)
	}
	if !transportAcceptsAuxiliary(alpha) || !transportAcceptsAuxiliary(beta) {
		t.Fatal("historical transport lost auxiliary capability after source change")
	}

	if err := os.WriteFile(configPath, []byte("Host *\n  User wildcard\n  Port 2200\nHost alpha\n  HostName alpha.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.refreshSources()
	if alpha.Draining || beta.SourceState != "deleted" || !beta.Draining {
		t.Fatalf("deleting beta did not preserve alpha and mark beta deleted: alpha=%+v beta=%+v", alpha, beta)
	}
}

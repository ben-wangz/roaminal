package connection

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSourceRevisionIsPerAliasAndDeterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh-projection")
	script := []byte("#!/bin/sh\nprintf 'host %s\\nuser coder\\n' \"$7\"\n")
	if err := os.WriteFile(path, script, 0o700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{sshPath: path}
	alpha, err := m.sourceRevision("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if alphaAgain, err := m.sourceRevision("alpha"); err != nil || alphaAgain != alpha {
		t.Fatalf("same alias fingerprint changed: %q / %v", alphaAgain, err)
	}
	beta, err := m.sourceRevision("beta")
	if err != nil {
		t.Fatal(err)
	}
	if beta == alpha {
		t.Fatal("different aliases should have different effective fingerprints")
	}
}

func TestSourceRevisionTracksEffectiveAliasOnly(t *testing.T) {
	realSSH, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh is unavailable")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	config := "Host *\n  User wildcard\n  Port 2200\nHost alpha\n  HostName alpha.example\nHost beta\n  HostName beta.example\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ssh-wrapper")
	script := fmt.Sprintf("#!/bin/sh\nexec %s -F %s \"$@\"\n", shellQuote(realSSH), shellQuote(configPath))
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{sshPath: path}
	alphaBefore, err := m.sourceRevision("alpha")
	if err != nil {
		t.Fatal(err)
	}
	betaBefore, err := m.sourceRevision("beta")
	if err != nil {
		t.Fatal(err)
	}
	if alphaBefore == betaBefore {
		t.Fatal("different effective Host projections should differ")
	}

	if err := os.WriteFile(configPath, []byte(config+"Host unrelated\n  HostName unrelated.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	alphaAfterUnrelated, err := m.sourceRevision("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if alphaAfterUnrelated != alphaBefore {
		t.Fatal("unrelated Host block changed alpha fingerprint")
	}

	changedBeta := "Host *\n  User wildcard\n  Port 2200\nHost alpha\n  HostName alpha.example\nHost beta\n  HostName beta-new.example\nHost unrelated\n  HostName unrelated.example\n"
	if err := os.WriteFile(configPath, []byte(changedBeta), 0o600); err != nil {
		t.Fatal(err)
	}
	alphaAfterBeta, err := m.sourceRevision("alpha")
	if err != nil {
		t.Fatal(err)
	}
	betaAfter, err := m.sourceRevision("beta")
	if err != nil {
		t.Fatal(err)
	}
	if alphaAfterBeta != alphaBefore {
		t.Fatal("beta Host block changed alpha fingerprint")
	}
	if betaAfter == betaBefore {
		t.Fatal("effective beta edit did not change beta fingerprint")
	}

	changedDefault := "Host *\n  User changed-default\n  Port 2200\nHost alpha\n  HostName alpha.example\nHost beta\n  HostName beta-new.example\nHost unrelated\n  HostName unrelated.example\n"
	if err := os.WriteFile(configPath, []byte(changedDefault), 0o600); err != nil {
		t.Fatal(err)
	}
	alphaAfterDefault, err := m.sourceRevision("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if alphaAfterDefault == alphaBefore {
		t.Fatal("effective wildcard/default edit did not change alpha fingerprint")
	}
}

package connection

import (
	"os"
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

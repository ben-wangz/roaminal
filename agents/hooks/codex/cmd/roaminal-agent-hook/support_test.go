package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallBinaryIfNeededRepairsOwnerPermissions(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "roaminal-agent-hook")
	if err := os.WriteFile(destination, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	expected, err := executableChecksum(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := installBinaryIfNeeded(destination, expected); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("got mode %o, want 0700", info.Mode().Perm())
	}
	actual, err := executableChecksum(destination)
	if err != nil || actual != expected {
		t.Fatalf("got checksum %q, want %q (err=%v)", actual, expected, err)
	}
}

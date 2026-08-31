package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
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

func TestEnsurePrivateDirRepairsPermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("got mode %o, want 0700", info.Mode().Perm())
	}
}

func TestInstallErrorCodeClassifiesStableFailures(t *testing.T) {
	for _, test := range []struct {
		message string
		want    string
	}{
		{message: "private directory permissions are unsafe", want: "private_directory_unsafe"},
		{message: "hooks file permissions are unsafe", want: "hooks_file_unsafe"},
		{message: "permission denied", want: "filesystem_permission_denied"},
		{message: "unexpected failure", want: "install_failed"},
	} {
		if got := installErrorCode(errors.New(test.message)); got != test.want {
			t.Fatalf("installErrorCode(%q) = %q, want %q", test.message, got, test.want)
		}
	}
}

func TestProbeResponseIncludesProviderForBackendVerification(t *testing.T) {
	response := probeResponse(model.ComponentConfig{ComponentVersion: "1", ComponentSHA256: "checksum"}, nil, nil)
	if response["provider"] != model.ProviderCodex {
		t.Fatalf("probe provider = %#v, want %q", response["provider"], model.ProviderCodex)
	}
	if response["componentVersion"] != "1" || response["componentSha256"] != "checksum" {
		t.Fatalf("probe component metadata = %#v", response)
	}
}

package sshkey

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func TestGenerationPromotesPairWithoutOverwrite(t *testing.T) {
	root, err := sshfs.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	inventory := New(root)
	request := GenerationRequest{Algorithm: "ed25519", FileName: "id_fixture_ed25519", Comment: "fixture"}
	paths, err := inventory.PrepareGeneration("11111111-1111-4111-8111-111111111111", request)
	if err != nil {
		t.Fatal(err)
	}
	command := inventory.GenerationCommand(paths, request)
	process := exec.Command(command[0], command[1:]...)
	process.Stdin = strings.NewReader("\n\n")
	if output, err := process.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v (%s)", err, output)
	}
	if err := inventory.Promote(paths); err != nil {
		t.Fatal(err)
	}
	keys := inventory.List()
	if len(keys) != 1 || keys[0].FileName != request.FileName || !keys[0].PublicKeyAvailable {
		t.Fatalf("unexpected inventory: %+v", keys)
	}
	if _, err := inventory.PrepareGeneration("22222222-2222-4222-8222-222222222222", request); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected no-overwrite error, got %v", err)
	}
}

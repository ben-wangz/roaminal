package sshkey

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func TestInventoryAcceptsReadOnlySymlinkWithinRoot(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is unavailable")
	}
	rootPath := t.TempDir()
	targetDir := filepath.Join(rootPath, "..data")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(targetDir, "id_project_ed25519")
	command := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-C", "fixture", "-f", targetPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v (%s)", err, output)
	}
	root, err := sshfs.OpenAt(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Symlink("..data/id_project_ed25519", filepath.Join(rootPath, "id_project_ed25519")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..data/id_project_ed25519.pub", filepath.Join(rootPath, "id_project_ed25519.pub")); err != nil {
		t.Fatal(err)
	}
	keys := New(root).List()
	if len(keys) != 1 {
		t.Fatalf("keys=%+v", keys)
	}
	if !keys[0].ReadOnly || !keys[0].PublicKeyAvailable || keys[0].Status != "available" {
		t.Fatalf("projected key metadata=%+v", keys[0])
	}
}

func TestInventoryRejectsEscapingKeySymlink(t *testing.T) {
	rootPath := t.TempDir()
	root, err := sshfs.OpenAt(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Symlink("/etc/passwd", filepath.Join(rootPath, "id_escape_rsa")); err != nil {
		t.Fatal(err)
	}
	if keys := New(root).List(); len(keys) != 0 {
		t.Fatalf("escaping key should be rejected: %+v", keys)
	}
}

func TestDeleteRemovesPrivateAndPublicPair(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is unavailable")
	}
	rootPath := t.TempDir()
	path := filepath.Join(rootPath, "id_remove_ed25519")
	if output, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v (%s)", err, output)
	}
	root, err := sshfs.OpenAt(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	inventory := New(root)
	if err := inventory.Delete(KeyID("id_remove_ed25519")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("private key still exists: %v", err)
	}
	if _, err := os.Stat(path + ".pub"); !os.IsNotExist(err) {
		t.Fatalf("public key still exists: %v", err)
	}
}

func TestDeleteRejectsProjectedKeySymlink(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootPath, "id_project_ed25519"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("id_project_ed25519", filepath.Join(rootPath, "id_link_ed25519")); err != nil {
		t.Fatal(err)
	}
	root, err := sshfs.OpenAt(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := New(root).Delete(KeyID("id_link_ed25519")); err == nil {
		t.Fatal("expected symlink deletion to be rejected")
	}
}

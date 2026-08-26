package definition

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func TestDefinitionServiceRecoversPreparedTransactionAfterRestart(t *testing.T) {
	sshDir := t.TempDir()
	root, err := sshfs.OpenAt(sshDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	initialConfig := []byte("# preserve\nHost alpha\n  HostName old.example\n")
	configPath := filepath.Join(sshDir, "config")
	if err := os.WriteFile(configPath, initialConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	store := connectionoptions.New(t.TempDir())
	if err := store.Save(map[string]connectionoptions.Tmux{"alpha": {Enabled: true, SessionName: "tmux-old", Pwd: "~/old"}}); err != nil {
		t.Fatal(err)
	}
	initialOptions, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	repo := sshconfig.New(root)
	service, err := New(repo, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	configSnapshot, optionsSnapshot, err := service.snapshots(true)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := newDefinitionTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeDefinitionTransaction(service.journalPath, transaction); err != nil {
		t.Fatal(err)
	}
	collection, err := repo.Collection(nil)
	if err != nil {
		t.Fatal(err)
	}
	newHost := "new.example"
	if _, err := repo.Update(collection.ETag, nil, "alpha", sshconfig.Edit{HostAlias: "alpha", HostName: &newHost}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAlias("alpha", connectionoptions.Tmux{Enabled: true, SessionName: "tmux-new", Pwd: "~/new"}, true); err != nil {
		t.Fatal(err)
	}

	if _, err := New(repo, nil, store); err != nil {
		t.Fatal(err)
	}
	recoveredConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	recoveredOptions, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredConfig, initialConfig) {
		t.Fatalf("prepared transaction did not restore SSH config: %s", recoveredConfig)
	}
	if !bytes.Equal(recoveredOptions, initialOptions) {
		t.Fatalf("prepared transaction did not restore options: %s", recoveredOptions)
	}
	if _, err := os.Stat(service.journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery left transaction journal: %v", err)
	}
}

func TestDefinitionServiceKeepsCommittedTransactionAfterRestart(t *testing.T) {
	sshDir := t.TempDir()
	root, err := sshfs.OpenAt(sshDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	configPath := filepath.Join(sshDir, "config")
	if err := os.WriteFile(configPath, []byte("Host alpha\n  HostName committed.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := connectionoptions.New(t.TempDir())
	if err := store.Save(map[string]connectionoptions.Tmux{"alpha": {Pwd: "~/committed"}}); err != nil {
		t.Fatal(err)
	}
	repo := sshconfig.New(root)
	service, err := New(repo, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	configSnapshot, optionsSnapshot, err := service.snapshots(true)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := newDefinitionTransaction(configSnapshot, optionsSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	transaction.Phase = definitionTransactionCommitted
	if err := writeDefinitionTransaction(service.journalPath, transaction); err != nil {
		t.Fatal(err)
	}
	if _, err := New(repo, nil, store); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(config, []byte("committed.example")) {
		t.Fatalf("committed transaction was rolled back: %s", config)
	}
	loaded, err := store.Load(nil)
	if err != nil || loaded.Options["alpha"].Pwd != "~/committed" {
		t.Fatalf("committed options changed after restart: %#v, %v", loaded.Options, err)
	}
	if _, err := os.Stat(service.journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed transaction journal was not cleaned: %v", err)
	}
}

func TestDefinitionServiceRollbackPreservesInvalidOptions(t *testing.T) {
	sshDir := t.TempDir()
	root, err := sshfs.OpenAt(sshDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	initialOptions := []byte("formatVersion: 2\nconnections:\n  alpha:\n    tmux:\n      enabled: true\n      enabled: false\n")
	store := connectionoptions.New(t.TempDir())
	if err := os.WriteFile(store.Path(), initialOptions, 0o600); err != nil {
		t.Fatal(err)
	}
	repo := sshconfig.New(root)
	service, err := New(repo, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := repo.Collection(nil)
	if err != nil {
		t.Fatal(err)
	}
	host := "example.test"
	if _, err := service.Create(collection.ETag, sshconfig.Edit{HostAlias: "alpha", HostName: &host}, &sshconfig.TmuxOptions{Enabled: true, SessionName: "tmux-alpha"}, &sshconfig.FileSystemOptions{Pwd: "~/alpha"}); err == nil {
		t.Fatal("invalid options source should reject the mutation")
	}
	if _, err := os.Stat(filepath.Join(sshDir, "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed mutation left SSH config behind: %v", err)
	}
	recoveredOptions, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recoveredOptions, initialOptions) {
		t.Fatalf("failed mutation changed invalid options source: %s", recoveredOptions)
	}
	if _, err := os.Stat(service.journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed mutation left transaction journal: %v", err)
	}
}

func TestConfigOnlyMutationSnapshotsExistingOptions(t *testing.T) {
	sshDir := t.TempDir()
	root, err := sshfs.OpenAt(sshDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host alpha\n  HostName old.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := connectionoptions.New(t.TempDir())
	if err := store.Save(map[string]connectionoptions.Tmux{"alpha": {Pwd: "~/alpha"}}); err != nil {
		t.Fatal(err)
	}
	service, err := New(sshconfig.New(root), nil, store)
	if err != nil {
		t.Fatal(err)
	}
	_, optionsSnapshot, err := service.snapshots(service.options != nil)
	if err != nil {
		t.Fatal(err)
	}
	if !optionsSnapshot.Exists || !bytes.Contains(optionsSnapshot.Data, []byte("~/alpha")) {
		t.Fatalf("config-only transaction did not capture options: %+v", optionsSnapshot)
	}
}

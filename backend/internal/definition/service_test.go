package definition

import (
	"errors"
	"os"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func TestDefinitionMutationsPreserveOptionsAcrossRenameCopyAndDelete(t *testing.T) {
	root, err := sshfs.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	repo := sshconfig.New(root)
	store := connectionoptions.New(t.TempDir())
	service, err := New(repo, nil, store)
	if err != nil {
		t.Fatal(err)
	}

	collection, err := repo.Collection(nil)
	if err != nil {
		t.Fatal(err)
	}
	host := "example.test"
	if _, err := service.Create(collection.ETag, sshconfig.Edit{HostAlias: "rollback", HostName: &host}, &sshconfig.TmuxOptions{Enabled: true, SessionName: "invalid name"}, &sshconfig.FileSystemOptions{Pwd: "~/rollback"}); err == nil {
		t.Fatal("invalid options write should fail")
	}
	if afterRollback, err := repo.Collection(nil); err != nil || len(afterRollback.Definitions) != 1 {
		t.Fatalf("config mutation was not rolled back: definitions=%d err=%v", len(afterRollback.Definitions), err)
	}
	collection, err = service.Create(collection.ETag, sshconfig.Edit{HostAlias: "alpha", HostName: &host}, &sshconfig.TmuxOptions{Enabled: true, SessionName: "tmux-a"}, &sshconfig.FileSystemOptions{Pwd: "~/alpha"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(nil)
	if err != nil || loaded.Options["alpha"].SessionName != "tmux-a" {
		t.Fatalf("create did not persist options: %#v, %v", loaded.Options, err)
	}

	collection, err = service.Update(collection.ETag, "alpha", sshconfig.Edit{HostAlias: "beta", HostName: &host}, &sshconfig.TmuxOptions{Enabled: true, SessionName: "tmux-b"}, &sshconfig.FileSystemOptions{Pwd: "~/beta"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(nil)
	if err != nil || loaded.Options["beta"].SessionName != "tmux-b" || loaded.Options["beta"].Pwd != "~/beta" {
		t.Fatalf("rename lost options: %#v, %v", loaded.Options, err)
	}
	if _, ok := loaded.Options["alpha"]; ok {
		t.Fatal("rename retained the old alias")
	}

	collection, err = service.Duplicate(collection.ETag, "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(nil)
	if err != nil || loaded.Options["gamma"].Pwd != "~/beta" || loaded.Options["gamma"].SessionName != "tmux-b" {
		t.Fatalf("duplicate did not copy options: %#v, %v", loaded.Options, err)
	}
	if _, err := service.Delete(collection.ETag, "gamma"); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Options["gamma"]; ok {
		t.Fatal("delete retained the deleted alias")
	}
	if _, err := os.Stat(service.journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed mutation left transaction journal: %v", err)
	}
}

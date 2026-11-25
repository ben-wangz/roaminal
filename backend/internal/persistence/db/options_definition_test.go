package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

const (
	oldSSHConfig      = "# preserve\nHost alpha\n  HostName alpha.example\n"
	fixtureMutationID = "0123456789abcdef0123456789abcdef"
)

func definitionFixture(t *testing.T) (*sshfs.Root, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(oldSSHConfig), 0o640); err != nil {
		t.Fatal(err)
	}
	root, err := sshfs.OpenAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, path
}

func TestSQLDefinitionMutations(t *testing.T) {
	s, _ := sqliteStore(t)
	root, path := definitionFixture(t)
	repo, options := sshconfig.New(root), s.ConnectionOptions()
	if err := options.UpdateAlias("stale", connectionoptions.Tmux{Pwd: "~/stale"}, true); err != nil {
		t.Fatal(err)
	}
	service, err := definition.NewWithRepository(repo, nil, options, s.DefinitionMutations())
	if err != nil {
		t.Fatal(err)
	}
	start, err := service.Collection()
	if err != nil {
		t.Fatal(err)
	}
	edit := sshconfig.Edit{HostAlias: "beta"}
	if _, err := service.Create(start.ETag, edit, &sshconfig.TmuxOptions{Enabled: true, SessionName: "bad name"}, nil); !errors.Is(err, connectionoptions.ErrInvalidSessionName) {
		t.Fatalf("invalid options error: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != oldSSHConfig {
		t.Fatalf("invalid options changed SSH config: %q %v", data, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("invalid options changed SSH config mode: %v %v", info, err)
	}
	if _, err := service.Create(`"stale"`, edit, nil, nil); !errors.Is(err, sshconfig.ErrPreconditionFailed) {
		t.Fatalf("stale ETag accepted: %v", err)
	}
	collection, err := service.Create(start.ETag, edit, &sshconfig.TmuxOptions{Enabled: true, SessionName: "work"}, &sshconfig.FileSystemOptions{Pwd: "~/beta"})
	if err != nil {
		t.Fatal(err)
	}
	collection, err = service.Update(collection.ETag, "beta", sshconfig.Edit{HostAlias: "gamma"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	collection, err = service.Duplicate(collection.ETag, "gamma", "copy")
	if err != nil {
		t.Fatal(err)
	}
	collection, err = service.Delete(collection.ETag, "gamma")
	if err != nil {
		t.Fatal(err)
	}
	// Options-only edits with an identical file revision still commit normally.
	if _, err := service.Update(collection.ETag, "copy", sshconfig.Edit{}, nil, &sshconfig.FileSystemOptions{Pwd: "~/copy"}); err != nil {
		t.Fatal(err)
	}
	view, err := options.Load(nil)
	want := connectionoptions.Tmux{Enabled: true, SessionName: "work", Pwd: "~/copy"}
	if err != nil || len(view.Options) != 2 || view.Options["copy"] != want || view.Options["stale"].Pwd != "~/stale" {
		t.Fatalf("options after mutation: %+v, %v", view, err)
	}
	if _, exists, err := s.DefinitionMutations().Pending(); err != nil || exists {
		t.Fatalf("mutations left an intent: exists=%v err=%v", exists, err)
	}
}

func TestSQLDefinitionRecovery(t *testing.T) {
	for _, test := range []struct {
		name        string
		publish     bool
		changeFile  bool
		changeMode  bool
		wantOptions string
	}{
		{name: "old revision discards", wantOptions: "~/old"},
		{name: "new revision finalizes", publish: true, wantOptions: "~/new"},
		{name: "external edit fails closed", publish: true, changeFile: true, wantOptions: "~/old"},
		{name: "external chmod fails closed", publish: true, changeMode: true, wantOptions: "~/old"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, cfg := sqliteStore(t)
			root, configPath := definitionFixture(t)
			plan := prepareDefinitionFixture(t, s, root)
			if test.publish {
				if published, err := sshconfig.New(root).ApplyMutation(plan); err != nil || !published {
					t.Fatalf("publish: %v %v", published, err)
				}
			}
			if test.changeFile {
				if err := os.WriteFile(configPath, []byte("# externally edited\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if test.changeMode {
				if err := os.Chmod(configPath, 0o640); err != nil {
					t.Fatal(err)
				}
			}
			repo := sshconfig.New(root)
			revision, err := repo.MutationRevision()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			options, mutations := reopened.ConnectionOptions(), reopened.DefinitionMutations()
			_, err = definition.NewWithRepository(repo, nil, options, mutations)
			conflict := test.changeFile || test.changeMode
			if (conflict && !errors.Is(err, definition.ErrMutationConflict)) || (!conflict && err != nil) {
				t.Fatalf("recover: %v", err)
			}
			if live, err := repo.MutationRevision(); err != nil || live != revision {
				t.Fatalf("recovery changed the file revision: %+v, %v", live, err)
			}
			if _, exists, err := mutations.Pending(); err != nil || exists != conflict {
				t.Fatalf("pending status after recovery: exists=%v err=%v", exists, err)
			}
			view, err := options.Load(nil)
			if err != nil || view.Options["alpha"].Pwd != test.wantOptions {
				t.Fatalf("options after recovery: %+v, %v", view, err)
			}
		})
	}
}

func prepareDefinitionFixture(t *testing.T, store *Store, root *sshfs.Root) sshconfig.PreparedMutation {
	t.Helper()
	if err := store.ConnectionOptions().UpdateAlias("alpha", connectionoptions.Tmux{Pwd: "~/old"}, true); err != nil {
		t.Fatal(err)
	}
	repo := sshconfig.New(root)
	collection, err := repo.Collection(nil)
	if err != nil {
		t.Fatal(err)
	}
	newHost := "edited.example"
	plan, err := repo.PrepareMutation(collection.ETag, nil, sshconfig.UpdateChange("alpha", sshconfig.Edit{HostName: &newHost}))
	if err != nil {
		t.Fatal(err)
	}
	intent := definition.MutationIntent{ID: fixtureMutationID, ETag: plan.ETag, Before: plan.Before, After: plan.After,
		Updates: []definition.OptionUpdate{{Alias: "alpha", Value: &connectionoptions.Tmux{Pwd: "~/new"}}}}
	if err := store.DefinitionMutations().Prepare(intent); err != nil {
		t.Fatal(err)
	}
	return plan
}

package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
	"gorm.io/gorm"
)

func TestSQLiteOptionsOnlyDefinitionUpdate(t *testing.T) {
	s, _ := sqliteStore(t)
	checkOptionsOnlyDefinitionUpdate(t, s)
}

func TestPostgresOptionsOnlyDefinitionUpdate(t *testing.T) {
	s, _ := postgresStore(t)
	checkOptionsOnlyDefinitionUpdate(t, s)
}

func checkOptionsOnlyDefinitionUpdate(t *testing.T, s *Store) {
	t.Helper()
	root, path := definitionFixture(t)
	// Like a projected Secret: the file is readable, but its symlink must
	// never be replaced. The target's mode and exact bytes must stay intact.
	target := path + ".projection"
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), path); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	options := s.ConnectionOptions()
	service, err := definition.NewWithRepository(sshconfig.New(root), nil, options, s.DefinitionMutations())
	if err != nil {
		t.Fatal(err)
	}
	start, err := service.Collection()
	if err != nil || !start.ConfigSource.Readable || start.ConfigSource.Writable {
		t.Fatalf("expected a readable, read-only source: %+v %v", start.ConfigSource, err)
	}
	if err := s.DB.Callback().Create().Before("gorm:create").Register("test:no-options-only-intent", func(tx *gorm.DB) {
		if tx.Statement.Table == (definitionMutationRow{}).TableName() {
			t.Error("options-only request attempted to persist a native intent")
			_ = tx.AddError(errors.New("unexpected definition mutation intent"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	host := "alpha.example"
	edit := sshconfig.Edit{HostAlias: "alpha", HostName: &host, IdentityFileNames: []string{}}
	tmux := &sshconfig.TmuxOptions{Enabled: true, SessionName: "work"}
	pwd := &sshconfig.FileSystemOptions{Pwd: "~/project"}
	if _, err := service.Update("", "alpha", edit, tmux, pwd); !errors.Is(err, sshconfig.ErrPreconditionRequired) {
		t.Fatalf("missing ETag: %v", err)
	}
	updated, err := service.Update(start.ETag, "alpha", edit, tmux, pwd)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ETag == start.ETag {
		t.Fatal("options-only edit did not invalidate the old collection ETag")
	}
	if _, err := service.Update(start.ETag, "alpha", edit, nil, &sshconfig.FileSystemOptions{Pwd: "~/stale"}); !errors.Is(err, sshconfig.ErrPreconditionFailed) {
		t.Fatalf("stale option edit error: %v", err)
	}
	newHost := "changed.example"
	if _, err := service.Update(updated.ETag, "alpha", sshconfig.Edit{HostName: &newHost}, nil, pwd); !errors.Is(err, sshfs.ErrNotWritable) {
		t.Fatalf("native edit must still require a writable config: %v", err)
	}
	if _, err := service.Update(updated.ETag, "missing", sshconfig.Edit{}, tmux, pwd); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("option edit for unknown definition: %v", err)
	}
	view, err := options.Load(nil)
	want := connectionoptions.Tmux{Enabled: true, SessionName: "work", Pwd: "~/project"}
	if err != nil || len(view.Options) != 1 || view.Options["alpha"] != want {
		t.Fatalf("options after update: %+v, %v", view.Options, err)
	}
	if link, err := os.Readlink(path); err != nil || link != filepath.Base(target) {
		t.Fatalf("options changed native config symlink: %q, %v", link, err)
	}
	if info, err := os.Stat(path); err != nil || !os.SameFile(info, before) || info.Mode() != before.Mode() {
		t.Fatalf("options changed native config metadata: %v, %v", info, err)
	}
	if bytes, err := os.ReadFile(path); err != nil || string(bytes) != oldSSHConfig {
		t.Fatalf("options changed native config contents: %q, %v", bytes, err)
	}
}

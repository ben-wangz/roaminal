package db

import (
	"errors"
	"os"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/definition"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"gorm.io/gorm"
)

func TestSQLDefinitionRecoveryAfterFinalizeFailure(t *testing.T) {
	s, _ := sqliteStore(t)
	root, path := definitionFixture(t)
	repo := sshconfig.New(root)
	mutations := s.DefinitionMutations()
	service, err := definition.NewWithRepository(repo, nil, s.ConnectionOptions(), mutations)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepareDefinitionFixture(t, s, root)
	callbacks := s.DB.Callback().Delete()
	const reject = "test:reject-intent-deletion"
	if err := callbacks.Before("gorm:delete").Register(reject, func(tx *gorm.DB) {
		if tx.Statement.Table == (definitionMutationRow{}).TableName() {
			_ = tx.AddError(errors.New("injected intent deletion failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := mutations.Resolve(fixtureMutationID, func() (bool, error) {
		return repo.ApplyMutation(plan)
	}); err == nil {
		t.Fatal("intent deletion failure did not abort the database transaction")
	}
	view, err := s.ConnectionOptions().Load(nil)
	if err != nil || view.Options["alpha"].Pwd != "~/old" {
		t.Fatalf("failed finalize changed the options: %+v, %v", view, err)
	}
	if _, exists, err := mutations.Pending(); err != nil || !exists {
		t.Fatalf("failed finalize lost the intent: exists=%v err=%v", exists, err)
	}
	fingerprint := func(string) (string, error) {
		data, err := os.ReadFile(path)
		return string(data), err
	}
	if _, err := service.ResolveSSH("alpha", func(string) (string, error) {
		t.Error("launch probed the published SSH config with uncommitted options")
		return "unexpected", nil
	}); !errors.Is(err, definition.ErrUnavailable) {
		t.Fatalf("runtime resolution during failed finalize: %v", err)
	}
	if _, err := service.Load(map[string]bool{"alpha": true}); !errors.Is(err, connectionoptions.ErrOptionsUnavailable) {
		t.Fatalf("FileSystem options during failed finalize: %v", err)
	}
	if err := callbacks.Remove(reject); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.ResolveSSH("alpha", fingerprint)
	want := "# preserve\nHost alpha\n  HostName edited.example\n"
	if err != nil || resolved.SourceRevision != want || resolved.Option.Pwd != "~/new" {
		t.Fatalf("runtime view after recovery: %+v, %v", resolved, err)
	}
	view, err = s.ConnectionOptions().Load(nil)
	if err != nil || view.Options["alpha"].Pwd != "~/new" {
		t.Fatalf("retry did not finalize the options: %+v, %v", view, err)
	}
}

func TestRuntimeDefinitionRejectsAnEditDuringTheEffectiveProbe(t *testing.T) {
	s, _ := sqliteStore(t)
	root, path := definitionFixture(t)
	service, err := definition.NewWithRepository(sshconfig.New(root), nil, s.ConnectionOptions(), s.DefinitionMutations())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ResolveSSH("alpha", func(string) (string, error) {
		return "new-ssh-revision", os.WriteFile(path, []byte("Host alpha\n  HostName changed.example\n"), 0o640)
	})
	if !errors.Is(err, definition.ErrUnavailable) || !errors.Is(err, sshconfig.ErrPreconditionFailed) {
		t.Fatalf("an external edit mixed with the previous read: %v", err)
	}
}

func TestSQLDefinitionDoesNotClobberAnExternalEditBeforePublish(t *testing.T) {
	s, _ := sqliteStore(t)
	root, path := definitionFixture(t)
	plan := prepareDefinitionFixture(t, s, root)
	repo := sshconfig.New(root)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if published, err := repo.ApplyMutation(plan); published || !errors.Is(err, sshconfig.ErrPreconditionFailed) {
		t.Fatalf("stale plan published=%v err=%v", published, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != oldSSHConfig {
		t.Fatalf("stale plan changed config: %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("stale plan changed config mode: %+v, %v", info, err)
	}
}

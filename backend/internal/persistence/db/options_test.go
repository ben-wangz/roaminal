package db

import (
	"context"
	"errors"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
)

func TestSQLConnectionOptionsPreserveStaleAliases(t *testing.T) {
	s, cfg := sqliteStore(t)
	options := s.ConnectionOptions()
	alpha := connectionoptions.Tmux{Enabled: true, SessionName: "work", Pwd: "~/alpha"}
	stale := connectionoptions.Tmux{Pwd: "/srv/stale"}
	if err := options.Replace(context.Background(), map[string]connectionoptions.Tmux{"alpha": alpha, "stale": stale}); err != nil {
		t.Fatal(err)
	}
	filtered, err := options.Load(map[string]bool{"alpha": true})
	if err != nil || len(filtered.Options) != 1 || filtered.Options["alpha"] != alpha {
		t.Fatalf("filtered options: %+v, %v", filtered, err)
	}
	if err := options.MoveAlias("alpha", "beta"); err != nil {
		t.Fatal(err)
	}
	if err := options.CopyAlias("beta", "gamma"); err != nil {
		t.Fatal(err)
	}
	if err := options.RemoveAlias("beta"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	view, err := reopened.ConnectionOptions().Load(nil)
	if err != nil || len(view.Options) != 2 || view.Options["gamma"] != alpha || view.Options["stale"] != stale {
		t.Fatalf("persisted options: %+v, %v", view, err)
	}
}

func TestSQLConnectionOptionsValidateBeforeReplacing(t *testing.T) {
	s, _ := sqliteStore(t)
	options := s.ConnectionOptions()
	if err := options.UpdateAlias("alpha", connectionoptions.Tmux{Enabled: true, SessionName: "work"}, true); err != nil {
		t.Fatal(err)
	}
	if err := options.UpdateAlias("alpha", connectionoptions.Tmux{Enabled: true, SessionName: "bad name"}, true); !errors.Is(err, connectionoptions.ErrInvalidSessionName) {
		t.Fatalf("invalid tmux session accepted: %v", err)
	}
	if err := options.Replace(context.Background(), map[string]connectionoptions.Tmux{"bad": {Pwd: "relative"}}); !errors.Is(err, connectionoptions.ErrInvalidPwd) {
		t.Fatalf("invalid FileSystem pwd accepted: %v", err)
	}
	view, err := options.Load(nil)
	if err != nil || len(view.Options) != 1 || view.Options["alpha"].Pwd != connectionoptions.DefaultPwd {
		t.Fatalf("invalid writes changed the options: %+v, %v", view, err)
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := (&Store{DB: tx, driver: s.driver}).ConnectionOptions().Replace(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	view, err = options.Load(nil)
	if err != nil || len(view.Options) != 1 {
		t.Fatalf("caller rollback did not preserve options: %+v, %v", view, err)
	}
}

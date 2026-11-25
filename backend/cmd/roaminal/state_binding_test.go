package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

func TestConcurrentStartupAllowsOnlyOneStateOwner(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	otherDir := filepath.Join(t.TempDir(), "state")
	store := openRuntimeStore(t, cfg, true)
	requireRuntimeSuccess(t, store.Close())

	type attempt struct {
		dir string
		err error
	}
	begin, results := make(chan struct{}), make(chan attempt, 2)
	for _, dir := range []string{stateDir, otherDir} {
		go func() {
			<-begin
			results <- attempt{dir, startDatabase(t, dir, cfg)}
		}()
	}
	close(begin)
	first, second := <-results, <-results
	winner, loser := first, second
	if first.err != nil {
		winner, loser = second, first
	}
	if winner.err != nil || (!errors.Is(loser.err, errDatabaseBinding) && !errors.Is(loser.err, db.ErrServingBinding)) {
		t.Fatalf("expected exactly one successful owner: winner=%v loser=%v", winner.err, loser.err)
	}
	binding := readRuntimeBinding(t, winner.dir)
	installation := readRuntimeInstallation(t, cfg)
	if installation.ID != binding.ID || installation.ServingOwner != binding.Owner || !installation.Served {
		t.Fatal("installation does not match the winning state directory")
	}
	// Even a matching installation ID is not enough to claim an owned target.
	state, err := lockStateDir(loser.dir)
	requireRuntimeSuccess(t, err)
	foreign := binding
	foreign.Owner = strings.Repeat("1", 32)
	if foreign.Owner == binding.Owner {
		foreign.Owner = strings.Repeat("2", 32)
	}
	err = state.bind(foreign)
	state.close()
	requireRuntimeSuccess(t, err)
	if err := startDatabase(t, loser.dir, cfg); !errors.Is(err, db.ErrServingBinding) {
		t.Fatalf("same database ID with a different owner: %v", err)
	}
	requireRuntimeSuccess(t, startDatabase(t, winner.dir, cfg))
}

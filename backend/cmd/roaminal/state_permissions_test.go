package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPrepareStateReopensRestoredDatabase(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	binding := readRuntimeBinding(t, stateDir)
	seedRuntimeAuth(t, cfg)
	dirs := []string{stateDir, filepath.Dir(cfg.SQLitePath), filepath.Join(stateDir, "upload-staging")}
	requireRuntimeSuccess(t, os.Mkdir(dirs[2], 0o700))
	for _, dir := range dirs {
		requireRuntimeSuccess(t, os.Chmod(dir, os.ModeSetgid|0o770))
	}
	privateFiles := []string{cfg.SQLitePath, filepath.Join(stateDir, bindingName)}
	for _, path := range privateFiles {
		requireRuntimeSuccess(t, os.Chmod(path, 0o660))
	}
	if err := startDatabase(t, stateDir, cfg); err == nil {
		t.Fatal("startup accepted group-writable state")
	}
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "prepare-state", "--state-dir", stateDir, "--create-sqlite-dir"))
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "prepare-state", "--state-dir", stateDir))
	for _, dir := range dirs {
		assertStateMode(t, dir, 0o700)
	}
	for _, path := range privateFiles {
		assertStateMode(t, path, 0o600)
	}
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	assertRuntimeBinding(t, stateDir, binding)
	assertRuntimeAuth(t, cfg, true)
}

func TestPrepareStateOnlyChangesKnownPrivatePaths(t *testing.T) {
	volume := t.TempDir()
	state := filepath.Join(volume, "state")
	id := "10000000-0000-4000-8000-000000000000"
	private := []string{"2fa-secret", "auth-sessions.json", "definition-mutation.json", "state/2fa-secret", "state/auth-sessions.json",
		"state/messages.json", "agent-endpoints.json", "push-subscriptions.json", "notification-preferences.json",
		"state/connection-instances/" + id + "/metadata.json", "state/audit/connection-instances/" + id + "/terminal.snapshot",
		"sessions/" + id + ".json", "sessions/" + id + ".snapshot", "uploads/job-fixture.json",
		"db/roaminal.sqlite3-journal", "db/roaminal.sqlite3-wal", "db/roaminal.sqlite3-shm"}
	unchanged := []string{"ssh-connection-options.yaml", "db/other", "../ssh/config", "sessions/quarantine.json", "uploads/upload-staging/content"}
	for _, name := range append(append([]string{}, private...), unchanged...) {
		path := filepath.Join(state, name)
		requireRuntimeSuccess(t, os.MkdirAll(filepath.Dir(path), 0o700))
		requireRuntimeSuccess(t, os.WriteFile(path, []byte("fixture"), 0o600))
		requireRuntimeSuccess(t, os.Chmod(path, 0o660))
	}
	requireRuntimeSuccess(t, os.Chmod(filepath.Join(state, "state"), os.ModeSetgid|0o770))
	requireRuntimeSuccess(t, preparePrivateState(state, false, false))
	assertStateMode(t, filepath.Join(state, "db/roaminal.sqlite3-wal"), 0o660)
	requireRuntimeSuccess(t, preparePrivateState(state, true, false))
	assertStateMode(t, filepath.Join(state, "state/2fa-secret"), 0o660)
	requireRuntimeSuccess(t, preparePrivateState(state, true, true))
	assertStateMode(t, filepath.Join(state, "state"), 0o700)
	for _, name := range private {
		assertStateMode(t, filepath.Join(state, name), 0o600)
	}
	for _, name := range unchanged {
		assertStateMode(t, filepath.Join(state, name), 0o660)
	}
}

func TestPrepareStateRefusesUnsafePaths(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "directory link", "root link", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			state, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, bindingName)
			requireRuntimeSuccess(t, os.WriteFile(target, []byte("untouched"), 0o640))
			requireRuntimeSuccess(t, os.Chmod(target, 0o640))
			path := filepath.Join(state, bindingName)
			switch kind {
			case "symlink":
				requireRuntimeSuccess(t, os.Symlink(target, path))
			case "hardlink":
				requireRuntimeSuccess(t, os.Link(target, path))
			case "directory link":
				requireRuntimeSuccess(t, os.Symlink(outside, filepath.Join(state, "db")))
			case "root link":
				state = filepath.Join(state, "state")
				requireRuntimeSuccess(t, os.Symlink(outside, state))
			case "fifo":
				requireRuntimeSuccess(t, syscall.Mkfifo(path, 0o600))
			}
			if err := preparePrivateState(state, true, false); !errors.Is(err, errUnsafeStatePermissions) {
				t.Fatalf("accepted unsafe %s: %v", kind, err)
			}
			assertStateMode(t, target, 0o640)
			if string(readRuntimeFile(t, target)) != "untouched" {
				t.Fatal("changed a path outside state")
			}
		})
	}
}

func TestPrepareStateRequiresOfflineStateLock(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	lock, err := lockStateDir(state)
	requireRuntimeSuccess(t, err)
	defer lock.close()
	if err := preparePrivateState(state, true, false); !errors.Is(err, errStateLocked) {
		t.Fatalf("modified state while locked: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created db directory while locked: %v", err)
	}
}

func assertStateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	requireRuntimeSuccess(t, err)
	if got := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky); got != want {
		t.Fatalf("%s got mode %v, want %v", path, got, want)
	}
}

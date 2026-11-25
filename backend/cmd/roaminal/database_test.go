package main

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

func TestFreshDatabaseStartupDefaultsToSQLite(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	if cfg.Driver != db.SQLite || cfg.SQLitePath != filepath.Join(stateDir, "db", "roaminal.sqlite3") {
		t.Fatalf("unexpected default database config: %+v", cfg)
	}
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	binding := readRuntimeBinding(t, stateDir)
	if !binding.valid() || binding.Driver != db.SQLite || binding.TransferSourceID != "" {
		t.Fatal("startup did not persist the SQLite binding")
	}
	for _, path := range []string{cfg.SQLitePath, filepath.Dir(cfg.SQLitePath), filepath.Join(stateDir, bindingName)} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("database path is not private: %s: %v", path, err)
		}
	}
	installation := readRuntimeInstallation(t, cfg)
	if installation.ID != binding.ID || installation.ServingOwner != binding.Owner || !installation.Served {
		t.Fatal("startup did not claim the installation for the local state")
	}
	assertRuntimeBinding(t, stateDir, binding)
}

func TestBoundStartupDoesNotRecreateOrChangeTarget(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	binding := readRuntimeBinding(t, stateDir)

	t.Run("missing", func(t *testing.T) {
		missing := sqliteAt(filepath.Join(t.TempDir(), "missing"))
		if err := startDatabase(t, stateDir, missing); err == nil {
			t.Fatal("startup accepted a missing bound database")
		}
		if _, err := os.Lstat(filepath.Dir(missing.SQLitePath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("startup created the destination directory: %v", err)
		}
		requireRuntimeSuccess(t, os.Remove(cfg.SQLitePath))
		if err := startDatabase(t, stateDir, cfg); err == nil {
			t.Fatal("startup accepted the deleted bound database")
		}
		if _, err := os.Lstat(cfg.SQLitePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("startup recreated the deleted database: %v", err)
		}
	})
	t.Run("different driver", func(t *testing.T) {
		// A binding mismatch must win even before attempting a database open.
		if err := startDatabase(t, stateDir, db.Config{Driver: db.Postgres}); !errors.Is(err, errDatabaseBinding) {
			t.Fatalf("changed driver: %v", err)
		}
	})
	t.Run("different installation", func(t *testing.T) {
		foreign := sqliteAt(filepath.Join(t.TempDir(), "private"))
		store := openRuntimeStore(t, foreign, true)
		var journal string
		requireRuntimeSuccess(t, store.SQL.QueryRow("PRAGMA journal_mode=WAL").Scan(&journal))
		if journal != "wal" {
			t.Fatalf("unexpected fixture journal mode: %s", journal)
		}
		requireRuntimeSuccess(t, store.Close())
		before := readRuntimeFile(t, foreign.SQLitePath)
		if err := startDatabase(t, stateDir, foreign); !errors.Is(err, errDatabaseBinding) {
			t.Fatalf("wrong installation: %v", err)
		}
		if !bytes.Equal(before, readRuntimeFile(t, foreign.SQLitePath)) {
			t.Fatal("startup changed the foreign SQLite database")
		}
	})
	assertRuntimeBinding(t, stateDir, binding)
}

func TestBoundStartupRefusesIdentityLessSchemaUpgrade(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	binding := readRuntimeBinding(t, stateDir)
	old := sqliteAt(filepath.Join(t.TempDir(), "v1"))
	requireRuntimeSuccess(t, os.Mkdir(filepath.Dir(old.SQLitePath), 0o700))
	requireRuntimeSuccess(t, os.WriteFile(old.SQLitePath, nil, 0o600))
	conn, err := sql.Open("sqlite3", old.SQLitePath)
	requireRuntimeSuccess(t, err)
	_, err = conn.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME NOT NULL);
		INSERT INTO schema_migrations VALUES (1, CURRENT_TIMESTAMP);
		CREATE TABLE totp_enrollment (key TEXT PRIMARY KEY, format_version INTEGER NOT NULL,
			enrollment_id TEXT NOT NULL, secret TEXT NOT NULL, last_accepted_time_step INTEGER NOT NULL);
		CREATE TABLE auth_sessions (id TEXT PRIMARY KEY, password_fingerprint TEXT NOT NULL,
			enrollment_id TEXT NOT NULL, refresh_token_hash TEXT NOT NULL, created_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL, refresh_expires_at DATETIME NOT NULL,
			rotated_at DATETIME NOT NULL, user_agent TEXT NOT NULL);`)
	closeErr := conn.Close()
	requireRuntimeSuccess(t, err)
	requireRuntimeSuccess(t, closeErr)
	before := readRuntimeFile(t, old.SQLitePath)
	if err := startDatabase(t, stateDir, old); err == nil {
		t.Fatal("startup accepted schema v1 without a verifiable installation identity")
	}
	if !bytes.Equal(before, readRuntimeFile(t, old.SQLitePath)) {
		t.Fatal("startup migrated an unidentified database")
	}
	assertRuntimeBinding(t, stateDir, binding)
	// Confirm this is an upgradable predecessor when no binding exists.
	unbound := filepath.Join(t.TempDir(), "state")
	requireRuntimeSuccess(t, startDatabase(t, unbound, old))
	if upgraded := readRuntimeBinding(t, unbound); !upgraded.valid() || upgraded.ID == binding.ID {
		t.Fatal("unbound startup did not establish a new installation identity")
	}
}

func TestStartupChecksLegacyFingerprintOnlyBeforeCutover(t *testing.T) {
	stateDir, cfg := sqliteRuntime(t)
	path := putRuntimeLegacyFiles(t, stateDir)
	if err := startDatabase(t, stateDir, cfg); !errors.Is(err, db.ErrImportRequired) {
		t.Fatalf("first startup without a completed import: %v", err)
	}
	assertRuntimeBinding(t, stateDir, databaseBinding{})
	if readRuntimeInstallation(t, cfg).Served {
		t.Fatal("failed startup marked the database as served")
	}
	importRuntimeLegacyFiles(t)
	original := readRuntimeFile(t, path)
	requireRuntimeSuccess(t, os.WriteFile(path, append(bytes.Clone(original), []byte("\n# changed after import\n")...), 0o600))
	if err := startDatabase(t, stateDir, cfg); !errors.Is(err, db.ErrImportRequired) {
		t.Fatalf("first startup accepted a stale fingerprint: %v", err)
	}
	assertRuntimeBinding(t, stateDir, databaseBinding{})

	requireRuntimeSuccess(t, os.WriteFile(path, original, 0o600))
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	binding := readRuntimeBinding(t, stateDir)
	requireRuntimeSuccess(t, os.WriteFile(path, []byte("invalid legacy options"), 0o600))
	requireRuntimeSuccess(t, startDatabase(t, stateDir, cfg))
	assertRuntimeBinding(t, stateDir, binding)
}

func sqliteRuntime(t *testing.T) (string, db.Config) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"ROAMINAL_DB_DRIVER", "ROAMINAL_DB_SQLITE_PATH", "ROAMINAL_DB_POSTGRES_DSN"} {
		t.Setenv(key, "")
	}
	stateDir, err := defaultStateDir()
	requireRuntimeSuccess(t, err)
	cfg, err := db.FromEnv(stateDir)
	requireRuntimeSuccess(t, err)
	return stateDir, cfg
}

func sqliteAt(dir string) db.Config {
	return db.Config{Driver: db.SQLite, SQLitePath: filepath.Join(dir, "roaminal.sqlite3")}
}

// Exercise the same locked open and preparation as the serving path, without
// starting the worker or listening on an HTTP port.
func startDatabase(t *testing.T, stateDir string, cfg db.Config) error {
	t.Helper()
	state, err := lockStateDir(stateDir)
	if err != nil {
		return err
	}
	defer state.close()
	binding, err := state.binding()
	if err != nil {
		return err
	}
	store, err := openServingDatabase(t.Context(), cfg, binding)
	if err != nil {
		return err
	}
	defer store.Close()
	return prepareServingDatabase(t.Context(), state, stateDir, filepath.Join(stateDir, "ssh", "config"), cfg.Driver, store)
}

func readRuntimeBinding(t *testing.T, stateDir string) databaseBinding {
	t.Helper()
	state, err := lockStateDir(stateDir)
	requireRuntimeSuccess(t, err)
	defer state.close()
	binding, err := state.binding()
	requireRuntimeSuccess(t, err)
	return binding
}

func assertRuntimeBinding(t *testing.T, stateDir string, want databaseBinding) {
	t.Helper()
	if got := readRuntimeBinding(t, stateDir); got != want {
		t.Fatal("local database binding unexpectedly changed")
	}
}

func openRuntimeStore(t *testing.T, cfg db.Config, create bool) *db.Store {
	t.Helper()
	open := db.OpenExisting
	if create {
		open = db.Open
	}
	store, err := open(t.Context(), cfg)
	requireRuntimeSuccess(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func readRuntimeInstallation(t *testing.T, cfg db.Config) db.Installation {
	t.Helper()
	store := openRuntimeStore(t, cfg, false)
	defer store.Close()
	result, err := store.Installation(t.Context())
	requireRuntimeSuccess(t, err)
	return result
}

func putRuntimeLegacyFiles(t *testing.T, stateDir string) string {
	t.Helper()
	requireRuntimeSuccess(t, os.MkdirAll(stateDir, 0o700))
	path := filepath.Join(stateDir, "ssh-connection-options.yaml")
	requireRuntimeSuccess(t, os.WriteFile(path, []byte("formatVersion: 2\nconnections: {}\n"), 0o600))
	return path
}

func importRuntimeLegacyFiles(t *testing.T) {
	t.Helper()
	copyDir := t.TempDir()
	putRuntimeLegacyFiles(t, copyDir)
	requireRuntimeSuccess(t, runtimeMaintenance(t, "db", "import-files", "--source", copyDir, "--apply"))
}

func readRuntimeFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	requireRuntimeSuccess(t, err)
	return data
}

func requireRuntimeSuccess(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

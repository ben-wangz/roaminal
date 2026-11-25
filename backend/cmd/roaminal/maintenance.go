package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ben-wangz/roaminal/backend/internal/dbbootstrap"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
)

const maintenanceUsage = `Usage:
  roaminal db import-files --source <copy> [--legacy-root <path>] [--ssh-config <path>] (--dry-run|--apply|--verify)
  roaminal db export --output <new-backup-file>
  roaminal db import --input <backup-file>
  roaminal db activate
  roaminal db prepare-state --state-dir <directory> [--create-sqlite-dir] [--repair-legacy-files]
  roaminal admin totp reset --confirm`

func runMaintenance(ctx context.Context, args []string, out io.Writer) error {
	if slices.Contains(args, "--help") || slices.Equal(args, []string{"db"}) || slices.Equal(args, []string{"admin"}) {
		_, err := fmt.Fprintln(out, maintenanceUsage)
		return err
	}
	if slices.Equal(args, []string{"db", "bootstrap-postgres"}) {
		if err := dbbootstrap.Run(ctx, dbbootstrap.FromEnv()); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "PostgreSQL bootstrap credentials verified")
		return err
	}
	if len(args) >= 2 && slices.Equal(args[:2], []string{"db", "prepare-state"}) {
		return prepareStateCommand(args[2:], out)
	}
	if len(args) >= 2 && args[0] == "db" && args[1] == "import-files" {
		return importLegacyFiles(ctx, args[2:], out)
	}
	if len(args) == 4 && slices.Equal(args[:3], []string{"admin", "totp", "reset"}) && args[3] == "--confirm" {
		return withOfflineDatabase(ctx, offlineBound, func(_ *stateLock, _ databaseBinding, _ db.Config, store *db.Store) error {
			if err := store.ResetEnrollment(ctx); err != nil {
				return err
			}
			_, err := fmt.Fprintln(out, "TOTP enrollment and existing login sessions reset")
			return err
		})
	}
	return runDatabaseTransfer(ctx, args, out)
}

func importLegacyFiles(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("db import-files", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	source := flags.String("source", "", "")
	root := flags.String("legacy-root", "", "")
	sshConfig := flags.String("ssh-config", "", "")
	dry := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	verify := flags.Bool("verify", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *source == "" || !oneMode(*dry, *apply, *verify) {
		return errors.New(maintenanceUsage)
	}
	loc, err := persistence.FindLegacyLocation(*source, *root)
	if err != nil {
		return err
	}
	data, err := persistence.ReadLegacy(loc, *sshConfig)
	if err != nil {
		return err
	}
	if !*dry {
		if err := applyOrVerifyFiles(ctx, loc, data, *apply); err != nil {
			return err
		}
	}
	mode := "checked (dry-run)"
	if !*dry {
		mode = "verified in database"
	}
	_, err = fmt.Fprintf(out, "Legacy state %s: fingerprint=%s enrollment=%t loginSessions=%d layouts=%d connectionInstances=%d uploads=%d endpoints=%d options=%d messages=%d subscriptions=%d preferences=%d\n",
		mode, data.Fingerprint, data.Enrollment != nil, len(data.Auth), len(data.Layouts), len(data.Instances), len(data.Uploads), len(data.Endpoints), len(data.Options), len(data.Messages), len(data.Subscriptions), len(data.Preferences))
	return err
}

func oneMode(modes ...bool) bool {
	count := 0
	for _, mode := range modes {
		if mode {
			count++
		}
	}
	return count == 1
}

func applyOrVerifyFiles(ctx context.Context, loc persistence.LegacyLocation, data persistence.LegacyData, apply bool) error {
	stateDir, err := defaultStateDir()
	if err != nil {
		return err
	}
	cfg, err := db.FromEnv(stateDir)
	if err != nil {
		return err
	}
	if cfg.Driver == db.SQLite && pathWithin(loc.StateDir, cfg.SQLitePath) {
		return errors.New("place the legacy copy outside the SQLite destination directory")
	}
	mode := offlineLegacyVerify
	if apply {
		mode = offlineLegacyApply
	}
	return withOfflineDatabase(ctx, mode, func(_ *stateLock, _ databaseBinding, _ db.Config, store *db.Store) error {
		if apply {
			return store.ImportLegacy(ctx, data)
		}
		return store.VerifyLegacy(ctx, data)
	})
}

func pathWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

type offlineDatabaseMode int

const (
	offlineBound offlineDatabaseMode = iota
	offlineActivate
	offlineLegacyVerify
	offlineLegacyApply
	offlineImport
)

func (mode offlineDatabaseMode) open(ctx context.Context, cfg db.Config, binding databaseBinding) (*db.Store, error) {
	switch mode {
	case offlineBound:
		return binding.openExisting(ctx, cfg, true)
	case offlineActivate:
		if binding.ID == "" {
			return nil, errDatabaseBinding
		}
	case offlineLegacyApply, offlineLegacyVerify:
		if binding.ID != "" {
			return binding.openExisting(ctx, cfg, false)
		}
		if mode == offlineLegacyApply {
			return db.Open(ctx, cfg)
		}
	case offlineImport:
		// This command intentionally prepares a separate destination; only
		// offline activation may change the local binding.
		return db.Open(ctx, cfg)
	}
	return db.OpenExisting(ctx, cfg)
}

func withOfflineDatabase(ctx context.Context, mode offlineDatabaseMode, run func(*stateLock, databaseBinding, db.Config, *db.Store) error) error {
	stateDir, err := defaultStateDir()
	if err != nil {
		return err
	}
	cfg, err := db.FromEnv(stateDir)
	if err != nil {
		return err
	}
	state, err := lockStateDir(stateDir)
	if err != nil {
		return err
	}
	defer state.close()
	binding, err := state.binding()
	if err != nil {
		return err
	}
	store, err := mode.open(ctx, cfg, binding)
	if err != nil {
		return err
	}
	defer store.Close()
	return run(state, binding, cfg, store)
}

func runDatabaseTransfer(ctx context.Context, args []string, out io.Writer) error {
	switch {
	case slices.Equal(args, []string{"db", "activate"}):
		return withOfflineDatabase(ctx, offlineActivate, func(state *stateLock, binding databaseBinding, cfg db.Config, store *db.Store) error {
			if err := activateTransferredDatabase(ctx, state, binding, cfg.Driver, store); err != nil {
				return err
			}
			_, err := fmt.Fprintln(out, "Database transfer binding active")
			return err
		})
	case len(args) == 4 && slices.Equal(args[:3], []string{"db", "export", "--output"}):
		return exportDatabase(ctx, args[3], out)
	case len(args) == 4 && slices.Equal(args[:3], []string{"db", "import", "--input"}):
		return importDatabase(ctx, args[3], out)
	default:
		return errors.New(maintenanceUsage)
	}
}

func exportDatabase(ctx context.Context, path string, out io.Writer) error {
	return withOfflineDatabase(ctx, offlineBound, func(_ *stateLock, _ databaseBinding, _ db.Config, store *db.Store) error {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return errors.New("backup output must be a new writable file")
		}
		exportErr := store.Export(ctx, f)
		syncErr := f.Sync()
		if closeErr := f.Close(); exportErr != nil || syncErr != nil || closeErr != nil {
			_ = os.Remove(path)
			if exportErr != nil {
				return exportErr
			}
			return errors.New("backup output could not be saved")
		}
		_, err = fmt.Fprintln(out, "Database backup written")
		return err
	})
}

func importDatabase(ctx context.Context, path string, out io.Writer) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("backup input must be a private regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return errors.New("open backup input failed")
	}
	defer file.Close()
	if current, err := file.Stat(); err != nil || !os.SameFile(info, current) {
		return errors.New("backup input changed while opening")
	}
	return withOfflineDatabase(ctx, offlineImport, func(_ *stateLock, _ databaseBinding, _ db.Config, store *db.Store) error {
		if err := store.Import(ctx, file); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "Database backup restored and verified")
		return err
	})
}

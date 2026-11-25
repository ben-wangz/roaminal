package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/persistence/db"
	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

const postgresStartupTimeout = time.Minute

func run(ctx context.Context, cfg config.Config) error {
	c, err := db.FromEnv(cfg.StateDir)
	if err != nil {
		return err
	}
	state, err := lockStateDir(cfg.StateDir)
	if err != nil {
		return err
	}
	defer state.close()
	binding, err := state.binding()
	if err != nil {
		return err
	}
	store, err := openServingDatabase(ctx, c, binding)
	if err != nil {
		return err
	}
	defer store.Close()
	sshRoot, sshErr := sshfs.Open()
	defer sshRoot.Close()
	if sshErr != nil {
		fmt.Fprintf(os.Stderr, "Roaminal SSH source unavailable: %v\n", sshErr)
	}
	if err := prepareServingDatabase(ctx, state, cfg.StateDir, filepath.Join(sshRoot.Name(), "config"), c.Driver, store); err != nil {
		return err
	}
	if err := filesystem.PrepareUploadStaging(cfg.StateDir); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Roaminal database driver=%s\n", c.Driver)
	return serve(ctx, cfg, store, sshRoot)
}

func openServingDatabase(ctx context.Context, c db.Config, binding databaseBinding) (*db.Store, error) {
	open := db.Open
	if binding.ID != "" {
		open = func(ctx context.Context, c db.Config) (*db.Store, error) {
			return binding.openExisting(ctx, c, false)
		}
	}
	if c.Driver == db.SQLite {
		return open(ctx, c)
	}
	ctx, cancel := context.WithTimeout(ctx, postgresStartupTimeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		store, err := open(ctx, c)
		if !errors.Is(err, db.ErrConnection) {
			return store, err
		}
		select {
		case <-ctx.Done():
			return nil, db.ErrConnection
		case <-ticker.C:
		}
	}
}

func (b databaseBinding) openExisting(ctx context.Context, cfg db.Config, requireServed bool) (*db.Store, error) {
	if b.ID == "" || b.Driver != cfg.Driver {
		return nil, errDatabaseBinding
	}
	// Schema v1 has no installation identity. Only accept a supported,
	// identifiable target; never fall back to the creating/migrating opener.
	store, err := db.OpenExisting(ctx, cfg)
	if err != nil {
		return nil, err
	}
	installation, err := store.Installation(ctx)
	if err == nil {
		err = b.check(cfg.Driver, installation)
		if err == nil && requireServed && !installation.Served {
			err = errDatabaseBinding
		}
	}
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func prepareServingDatabase(ctx context.Context, state *stateLock, stateDir, sshConfig, driver string, store *db.Store) error {
	installation, err := store.Installation(ctx)
	if err != nil {
		return err
	}
	binding, err := state.binding()
	if err != nil {
		return err
	}
	if binding.ID != "" {
		if err := binding.check(driver, installation); err != nil {
			return err
		}
	} else if installation.Served {
		return errDatabaseBinding
	}
	var legacyPresent bool
	var fingerprint string
	// After cutover, files are no longer authoritative. An explicit activation
	// carries that decision to the verified successor of the bound database.
	if !installation.Served && binding.TransferSourceID == "" {
		loc, err := persistence.FindLegacyLocation(stateDir, "")
		if err != nil {
			return fmt.Errorf("inspect legacy application state: %w", err)
		}
		legacyPresent = loc.Present
		if legacyPresent {
			if installation.LegacyFingerprint == "" {
				return db.ErrImportRequired
			}
			data, err := persistence.ReadLegacy(loc, sshConfig)
			if err != nil {
				return err
			}
			if data.Fingerprint != installation.LegacyFingerprint {
				return db.ErrImportRequired
			}
			fingerprint = data.Fingerprint
		}
	}
	if binding.ID == "" {
		// Do not publish the first local binding to a damaged logical restore.
		if installation.TransferSourceID != "" {
			if err := store.VerifyTransfer(ctx); err != nil {
				return err
			}
		}
		var owner [16]byte
		if _, err := rand.Read(owner[:]); err != nil {
			return errors.New("generate database owner identity failed")
		}
		binding = databaseBinding{Driver: driver, ID: installation.ID, Owner: hex.EncodeToString(owner[:])}
		// Publish locally first so a crash after the database claim is retryable.
		if err := state.bind(binding); err != nil {
			return err
		}
	}
	return store.PrepareServing(ctx, binding.Owner, legacyPresent, fingerprint)
}

func activateTransferredDatabase(ctx context.Context, state *stateLock, binding databaseBinding, targetDriver string, store *db.Store) error {
	installation, err := store.Installation(ctx)
	if err != nil {
		return err
	}
	if binding.ID == installation.ID && binding.TransferSourceID != "" {
		if err := binding.check(targetDriver, installation); err != nil || installation.Served {
			return err // An already serving database has legitimately changed.
		}
		return store.VerifyTransfer(ctx)
	}
	if binding.ID == "" || installation.Served || installation.TransferSourceID != binding.ID {
		return errDatabaseBinding
	}
	if err := store.VerifyTransfer(ctx); err != nil {
		return err
	}
	return state.bind(databaseBinding{
		Driver: targetDriver, ID: installation.ID, Owner: binding.Owner, TransferSourceID: binding.ID,
	})
}

func defaultStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("resolve home directory failed")
	}
	return filepath.Join(home, ".roaminal"), nil
}

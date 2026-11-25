package persistence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// LegacyLocation describes file-backed state without opening the old Store,
// whose startup migration can move or replace source material.
type LegacyLocation struct {
	StateDir string
	Root     string
	Present  bool
}

func FindLegacyLocation(stateDir, explicitRoot string) (LegacyLocation, error) {
	stateDir, err := filepath.Abs(stateDir)
	if err != nil {
		return LegacyLocation{}, err
	}
	child := filepath.Join(stateDir, "state")
	direct, err := legacyDataAt(stateDir)
	if err != nil {
		return LegacyLocation{}, err
	}
	nested, err := legacyDataAt(child)
	if err != nil {
		return LegacyLocation{}, err
	}
	if direct && nested {
		return LegacyLocation{}, ErrAmbiguousStateLayout
	}
	root := stateDir
	if nested {
		root = child
	}
	if explicitRoot != "" {
		explicitRoot, err = filepath.Abs(explicitRoot)
		if err != nil || (explicitRoot != stateDir && explicitRoot != child) || ((direct || nested) && explicitRoot != root) {
			return LegacyLocation{}, errors.New("legacy root must select the populated source directory or its state child")
		}
		root = explicitRoot
	}
	sidecars, err := containsLegacyFiles(stateDir, []string{"ssh-connection-options.yaml", "definition-mutation.json"})
	if err != nil {
		return LegacyLocation{}, err
	}
	return LegacyLocation{StateDir: stateDir, Root: root, Present: direct || nested || sidecars}, nil
}

var legacyRecordFiles = []string{
	"auth-sessions.json", "2fa-secret", "workspace-layouts.json", "agent-endpoints.json", "messages.json", "push-subscriptions.json", "notification-preferences.json",
}

func legacyDataAt(root string) (bool, error) {
	exists, err := legacyDir(root)
	if err != nil || !exists {
		return false, err
	}
	present, err := containsLegacyFiles(root, legacyRecordFiles)
	if err != nil {
		return false, err
	}
	if _, err := legacyDir(filepath.Join(root, "audit")); err != nil {
		return false, err
	}
	for _, rel := range []string{"connection-instances", filepath.Join("audit", "connection-instances"), "sessions", "migrations", "uploads"} {
		dir := filepath.Join(root, rel)
		entries, err := listLegacyDir(dir)
		if err != nil {
			return false, err
		}
		for _, entry := range entries {
			if rel != "uploads" || !isUploadStaging(entry) {
				present = true
			}
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".connection-instances-migration-") {
			return true, nil
		}
	}
	return present, nil
}

func containsLegacyFiles(dir string, names []string) (bool, error) {
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func legacyDir(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, errors.New("legacy source contains an unsafe directory")
	}
	return true, nil
}

func listLegacyDir(path string) ([]os.DirEntry, error) {
	exists, err := legacyDir(path)
	if err != nil || !exists {
		return nil, err
	}
	return os.ReadDir(path)
}

func (loc LegacyLocation) checkRecovery() error {
	for _, root := range []string{loc.StateDir, filepath.Join(loc.StateDir, "state")} {
		entries, err := listLegacyDir(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".connection-instances-migration-") {
				return errors.New("unfinished legacy connection migration: reconcile the backup before import")
			}
		}
	}
	entries, err := listLegacyDir(filepath.Join(loc.Root, "migrations"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		dir := filepath.Join(loc.Root, "migrations", entry.Name())
		if !entry.IsDir() {
			return errors.New("unexpected legacy migration artifact")
		}
		if present, err := containsLegacyFiles(dir, []string{"in-progress", "failed"}); err != nil || present {
			return errors.New("unfinished legacy migration: reconcile the backup before import")
		}
		if info, err := os.Lstat(filepath.Join(dir, "complete")); err != nil || !info.Mode().IsRegular() {
			return errors.New("legacy migration is missing a completion record")
		}
	}
	return nil
}

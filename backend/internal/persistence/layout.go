package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func New(root string) (*Store, error) { return newStore(root) }

func newStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	rootPrivateErr := ensurePrivateDirectory(root)
	if rootPrivateErr != nil && !errors.Is(rootPrivateErr, errWorldPermissions) {
		return nil, fmt.Errorf("prepare state directory: %w", rootPrivateErr)
	}
	childRoot := filepath.Join(root, "state")
	directHasData, err := stateRootHasData(root)
	if err != nil {
		return nil, fmt.Errorf("inspect direct state directory: %w", err)
	}
	childHasData, err := stateRootHasData(childRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect private state directory: %w", err)
	}
	stateRoot, layout := root, LayoutDirect
	if rootPrivateErr != nil {
		if directHasData {
			return nil, ErrAmbiguousStateLayout
		}
		stateRoot, layout = childRoot, LayoutPrivateChild
		if err := os.MkdirAll(stateRoot, 0o700); err != nil {
			return nil, fmt.Errorf("create private state directory: %w", err)
		}
		if err := ensurePrivateDirectory(stateRoot); err != nil {
			return nil, fmt.Errorf("prepare private state directory: %w", err)
		}
	} else if directHasData && childHasData {
		return nil, ErrAmbiguousStateLayout
	} else if !directHasData && childHasData {
		stateRoot, layout = childRoot, LayoutPrivateChild
		if err := ensurePrivateDirectory(stateRoot); err != nil {
			return nil, fmt.Errorf("prepare private state directory: %w", err)
		}
	}
	connectionsDir := filepath.Join(stateRoot, "connection-instances")
	auditDir := filepath.Join(stateRoot, "audit")
	diagnosticsDir := filepath.Join(stateRoot, "diagnostics")
	if err := os.MkdirAll(connectionsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create connection instances directory: %w", err)
	}
	if err := ensurePrivateDirectory(connectionsDir); err != nil {
		return nil, fmt.Errorf("prepare connection instances directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(auditDir, "connection-instances"), 0o700); err != nil {
		return nil, fmt.Errorf("create audit directory: %w", err)
	}
	if err := ensurePrivateDirectory(auditDir); err != nil {
		return nil, fmt.Errorf("prepare audit directory: %w", err)
	}
	if err := ensurePrivateDirectory(filepath.Join(auditDir, "connection-instances")); err != nil {
		return nil, fmt.Errorf("prepare audit connection directory: %w", err)
	}
	store := &Store{Root: stateRoot, ConnectionsDir: connectionsDir, AuditDir: auditDir, DiagnosticsDir: diagnosticsDir, Layout: layout, degradedIDs: make(map[string]struct{})}
	if err := store.migrateLegacySessions(); err != nil {
		return nil, err
	}
	return store, nil
}

func stateRootHasData(root string) (bool, error) {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, errors.New("state root is not a directory")
	}
	if _, err := os.Stat(filepath.Join(root, "auth-sessions.json")); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(root, "workspace-layouts.json")); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err == nil && len(entries) > 0 {
		return true, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	entries, err = os.ReadDir(filepath.Join(root, "connection-instances"))
	if errors.Is(err, os.ErrNotExist) {
		entries, err = os.ReadDir(filepath.Join(root, "diagnostics"))
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return len(entries) > 0, nil
	}
	if err != nil {
		return false, err
	}
	if len(entries) > 0 {
		return true, nil
	}
	entries, err = os.ReadDir(filepath.Join(root, "diagnostics"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err == nil {
		return nil
	} else if !os.IsPermission(err) {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	if info.Mode().Perm()&0o007 != 0 {
		return errWorldPermissions
	}
	probe, err := os.CreateTemp(path, ".roaminal-permission-*")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probeName)
		return fmt.Errorf("close write probe: %w", err)
	}
	if err := os.Remove(probeName); err != nil {
		return fmt.Errorf("remove write probe: %w", err)
	}
	return nil
}

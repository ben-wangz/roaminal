package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func prepareStateCommand(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("db prepare-state", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("state-dir", "", "")
	createSQLite := flags.Bool("create-sqlite-dir", false, "")
	repairLegacy := flags.Bool("repair-legacy-files", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !filepath.IsAbs(*path) || filepath.Clean(*path) == "/" {
		return errors.New(maintenanceUsage)
	}
	if err := preparePrivateState(filepath.Clean(*path), *createSQLite, *repairLegacy); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "Roaminal state permissions prepared")
	return err
}

// preparePrivateState only tightens known state paths. It is never called on
// an import source by the importer or on the mounted workspace or SSH data.
func preparePrivateState(path string, createSQLite, repairLegacy bool) error {
	parent, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return errors.New("open parent of Roaminal state directory failed")
	}
	defer parent.Close()
	name := filepath.Base(path)
	dir, err := openOwnedStatePath(parent, name, true)
	if err != nil {
		return statePermissionError(name, err)
	}
	defer dir.Close()
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errStateLocked
	}
	if err := setPrivateMode(dir, 0o700); err != nil {
		return statePermissionError(name, err)
	}
	root, err := checkedStateRoot(parent, name, dir)
	if err != nil {
		return statePermissionError(name, err)
	}
	defer root.Close()
	if err := prepareStateFiles(root, bindingName); err != nil {
		return err
	}
	if createSQLite {
		if err := root.Mkdir("db", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return statePermissionError("db", err)
		}
		if err := withPrivateStateSubdir(root, "db", func(dbRoot *os.Root) error {
			return prepareStateFiles(dbRoot, "roaminal.sqlite3", "roaminal.sqlite3-journal", "roaminal.sqlite3-wal", "roaminal.sqlite3-shm")
		}); err != nil {
			return err
		}
	}
	if err := withPrivateStateSubdir(root, "upload-staging", nil); err != nil {
		return err
	}
	if repairLegacy {
		return preparePrivateLegacyFiles(root)
	}
	return nil
}

func withPrivateStateSubdir(parent *os.Root, name string, prepare func(*os.Root) error) error {
	dir, err := openOwnedStatePath(parent, name, true)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return statePermissionError(name, err)
	}
	defer dir.Close()
	if err := setPrivateMode(dir, 0o700); err != nil {
		return statePermissionError(name, err)
	}
	if prepare == nil {
		return nil
	}
	root, err := checkedStateRoot(parent, name, dir)
	if err != nil {
		return statePermissionError(name, err)
	}
	defer root.Close()
	return prepare(root)
}

func prepareStateFiles(root *os.Root, names ...string) error {
	for _, name := range names {
		f, err := openOwnedStatePath(root, name, false)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return statePermissionError(name, err)
		}
		err = setPrivateMode(f, 0o600)
		_ = f.Close()
		if err != nil {
			return statePermissionError(name, err)
		}
	}
	return nil
}

func openOwnedStatePath(root *os.Root, name string, dir bool) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !ownedStatePath(info, dir) {
		return nil, errUnsafeStatePermissions
	}
	flags := os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if dir {
		flags |= syscall.O_DIRECTORY
	}
	f, err := root.OpenFile(name, flags, 0)
	if err != nil {
		return nil, err
	}
	current, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || !ownedStatePath(current, dir) {
		_ = f.Close()
		return nil, errUnsafeStatePermissions
	}
	return f, nil
}

func ownedStatePath(info os.FileInfo, dir bool) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && (dir && info.IsDir() || !dir && info.Mode().IsRegular() && stat.Nlink == 1)
}

func checkedStateRoot(parent *os.Root, name string, directory *os.File) (*os.Root, error) {
	root, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(".")
	opened, statErr := directory.Stat()
	if err != nil || statErr != nil || !os.SameFile(current, opened) {
		_ = root.Close()
		return nil, errUnsafeStatePermissions
	}
	return root, nil
}

func setPrivateMode(f *os.File, mode os.FileMode) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !ownedStatePath(info, mode == 0o700) {
		return errUnsafeStatePermissions
	}
	if info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == mode {
		return nil
	}
	return f.Chmod(mode)
}

var errUnsafeStatePermissions = errors.New("expected an owned directory or an owned, singly linked regular file")

func statePermissionError(name string, err error) error {
	return fmt.Errorf("prepare private state path %q: %w", name, err)
}

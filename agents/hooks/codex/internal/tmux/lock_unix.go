//go:build linux || darwin

package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func acquireTmuxLock(ctx context.Context, path string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensurePrivateDirectory(filepath.Dir(filepath.Dir(path))); err != nil {
		return func() {}, fmt.Errorf("prepare roaminal directory: %w", err)
	}
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return func() {}, fmt.Errorf("prepare tmux lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return func() {}, fmt.Errorf("open tmux lock: %w", err)
	}
	if err := validatePrivateLockFile(file, path); err != nil {
		_ = file.Close()
		return func() {}, err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = file.Close()
			return func() {}, fmt.Errorf("acquire tmux lock: %w", err)
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return func() {}, fmt.Errorf("acquire tmux lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		err = os.Mkdir(path, 0700)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("private directory is unsafe: %s", path)
	}
	if info.Mode().Perm() != 0700 {
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0700 {
			return fmt.Errorf("private directory permissions are unsafe: %s", path)
		}
	}
	return nil
}

func validatePrivateLockFile(file *os.File, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat tmux lock: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("tmux lock is unsafe: %s", path)
	}
	if info.Mode().Perm() != 0600 {
		if err := file.Chmod(0600); err != nil {
			return fmt.Errorf("repair tmux lock permissions: %w", err)
		}
	}
	return nil
}

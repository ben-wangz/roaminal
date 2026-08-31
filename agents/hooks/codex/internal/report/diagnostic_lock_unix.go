//go:build linux || darwin

package report

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func withDiagnosticFileLock(path string, fn func() error) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("diagnostic lock is unsafe: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("diagnostic lock is unsafe: %s", path)
	}
	if err := file.Chmod(0600); err != nil {
		return err
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			return fn()
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("acquire diagnostic lock: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("acquire diagnostic lock: timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

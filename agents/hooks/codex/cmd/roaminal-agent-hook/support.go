package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
)

func writeConfig(cfg model.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(configPath(), append(data, '\n'), 0600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return os.Mkdir(path, 0700)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return errors.New("private directory permissions are unsafe")
	}
	return nil
}

func installBinary(destination string) error {
	self, err := os.Open(os.Args[0])
	if err != nil {
		return err
	}
	defer self.Close()
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".hook-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0700); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, self); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(destination))
}

func installBinaryIfNeeded(destination, expectedChecksum string) error {
	info, err := os.Lstat(destination)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return errors.New("installed binary permissions are unsafe")
		}
		if info.Mode().Perm() != 0700 {
			return installBinary(destination)
		}
		checksum, checksumErr := executableChecksum(destination)
		if checksumErr != nil {
			return checksumErr
		}
		if checksum == expectedChecksum {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return installBinary(destination)
}

func acquireLock(path string, wait time.Duration) (func(), error) {
	owner := lockOwner{PID: os.Getpid(), ProcessStart: processStart(os.Getpid()), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if err := os.Mkdir(path, 0700); err == nil {
			metadata := filepath.Join(path, "owner")
			data, marshalErr := json.Marshal(owner)
			if marshalErr != nil {
				_ = os.Remove(path)
				return nil, marshalErr
			}
			if err := os.WriteFile(metadata, data, 0600); err != nil {
				_ = os.Remove(path)
				return nil, err
			}
			return func() { releaseLock(path, owner) }, nil
		} else if !os.IsExist(err) {
			return nil, err
		} else if staleLock(path) {
			_ = os.Remove(filepath.Join(path, "owner"))
			_ = os.Remove(path)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("installation lock timeout")
}

func staleLock(path string) bool {
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) < 2*time.Minute {
		return false
	}
	data, err := os.ReadFile(filepath.Join(path, "owner"))
	if err != nil {
		return true
	}
	var owner lockOwner
	if json.Unmarshal(data, &owner) != nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr != nil {
			return true
		}
		owner = lockOwner{PID: pid}
	}
	if owner.PID < 1 {
		return true
	}
	process, err := os.FindProcess(owner.PID)
	if err != nil {
		return true
	}
	err = process.Signal(syscall.Signal(0))
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return true
	}
	if owner.ProcessStart == "" {
		return false
	}
	currentStart := processStart(owner.PID)
	return currentStart != "" && currentStart != owner.ProcessStart
}

type lockOwner struct {
	PID          int    `json:"pid"`
	ProcessStart string `json:"processStart"`
	CreatedAt    string `json:"createdAt"`
}

func releaseLock(path string, owner lockOwner) {
	data, err := os.ReadFile(filepath.Join(path, "owner"))
	if err != nil {
		return
	}
	var current lockOwner
	if json.Unmarshal(data, &current) != nil || current.PID != owner.PID || current.ProcessStart != owner.ProcessStart {
		return
	}
	_ = os.Remove(filepath.Join(path, "owner"))
	_ = os.Remove(path)
}

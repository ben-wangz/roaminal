package imagepreview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ydylla/fcache"
)

const (
	markerName    = ".roaminal-image-preview-cache"
	markerContent = "roaminal filesystem image preview cache v1\n"
)

func validCachePath(value string) bool {
	if !filepath.IsAbs(value) {
		return false
	}
	clean := filepath.Clean(value)
	if clean == "/" || clean == "/tmp" || clean == "/workspace" || clean == "/home" || clean == "/home/roaminal" || clean == "/home/roaminal/.ssh" {
		return false
	}
	for _, reserved := range []string{"/tmp/", "/workspace/", "/home/roaminal/.ssh/"} {
		if strings.HasPrefix(clean, reserved) {
			return false
		}
	}
	return true
}

func prepareManagedDirectory(root string) (string, string, error) {
	if !validCachePath(root) {
		return "", "", errors.New("invalid cache directory")
	}
	if err := ensureNoSymlinkPath(root); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", "", err
	}
	if err := ensureNoSymlinkPath(root); err != nil {
		return "", "", err
	}
	marker := filepath.Join(root, markerName)
	data, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			return "", "", readErr
		}
		if len(entries) != 0 {
			return "", "", errors.New("cache directory is not Roaminal-owned")
		}
		if err := os.WriteFile(marker, []byte(markerContent), 0o600); err != nil {
			return "", "", err
		}
	} else if err != nil {
		return "", "", err
	} else if string(data) != markerContent {
		return "", "", errors.New("cache ownership marker is invalid")
	}
	dataDir := filepath.Join(root, "fcache-data")
	stagingDir := filepath.Join(root, "staging")
	for _, directory := range []string{dataDir, stagingDir} {
		if err := ensureNoSymlinkPath(directory); err != nil {
			return "", "", err
		}
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", "", err
		}
	}
	return dataDir, stagingDir, nil
}

func ensureNoSymlinkPath(value string) error {
	clean := filepath.Clean(value)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(clean, current), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains symlink: %s", current)
		}
	}
	return nil
}

func buildCache(directory string, targetBytes int64) (fcache.Cache, error) {
	return fcache.Builder(directory, fcache.Size(targetBytes)).WithEvictionInterval(0).WithFileMode(0o600).Build()
}

func replaceCacheData(directory string) error {
	if err := ensureNoSymlinkPath(directory); err != nil {
		return err
	}
	if filepath.Base(directory) != "fcache-data" {
		return errors.New("refusing to replace an unowned cache child")
	}
	if err := os.RemoveAll(directory); err != nil {
		return err
	}
	return os.MkdirAll(directory, 0o700)
}

func clearStaging(directory string) error {
	if err := ensureNoSymlinkPath(directory); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

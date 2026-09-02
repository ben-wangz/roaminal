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
	// Kubernetes emptyDir mounts are commonly root-owned even when the
	// application runs with a non-root UID. The marker and managed children
	// remain private, so an ownership-only chmod failure on the mount point is
	// safe to tolerate; all other filesystem errors still disable the cache.
	if err := os.Chmod(root, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return "", "", err
	}
	marker := filepath.Join(root, markerName)
	if err := ensureMarker(marker, root); err != nil {
		return "", "", err
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
		if err := ensureNoSymlinkPath(directory); err != nil {
			return "", "", err
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return "", "", err
		}
	}
	return dataDir, stagingDir, nil
}

func ensureMarker(marker, root string) error {
	for {
		info, err := os.Lstat(marker)
		if errors.Is(err, os.ErrNotExist) {
			entries, readErr := os.ReadDir(root)
			if readErr != nil {
				return readErr
			}
			if len(entries) != 0 {
				return errors.New("cache directory is not Roaminal-owned")
			}
			file, createErr := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if errors.Is(createErr, os.ErrExist) {
				continue
			}
			if createErr != nil {
				return createErr
			}
			written, writeErr := file.WriteString(markerContent)
			closeErr := file.Close()
			if writeErr != nil {
				return writeErr
			}
			if closeErr != nil {
				return closeErr
			}
			if written != len(markerContent) {
				return errors.New("cache ownership marker was partially written")
			}
			info, err = os.Lstat(marker)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("cache ownership marker must be a regular file")
		}
		data, readErr := os.ReadFile(marker)
		if readErr != nil {
			return readErr
		}
		latest, statErr := os.Lstat(marker)
		if statErr != nil {
			return statErr
		}
		if latest.Mode()&os.ModeSymlink != 0 || !latest.Mode().IsRegular() {
			return errors.New("cache ownership marker must be a regular file")
		}
		if string(data) != markerContent {
			return errors.New("cache ownership marker is invalid")
		}
		return os.Chmod(marker, 0o600)
	}
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
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(directory); err != nil {
		return err
	}
	return os.Chmod(directory, 0o700)
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

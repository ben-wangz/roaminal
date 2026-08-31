package report

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func privateDiagnosticFile(path string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("diagnostic log is unsafe: %s", path)
	}
	if info.Mode().Perm() != 0600 {
		if err := os.Chmod(path, 0600); err != nil {
			return nil, false, err
		}
		info, err = os.Lstat(path)
		if err != nil {
			return nil, false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return nil, false, fmt.Errorf("diagnostic log permissions are unsafe: %s", path)
		}
	}
	return info, true, nil
}

func validateDiagnosticFile(file *os.File, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("diagnostic log is unsafe: %s", path)
	}
	if info.Mode().Perm() != 0600 {
		if err := file.Chmod(0600); err != nil {
			return err
		}
	}
	return nil
}

func expireDiagnosticFile(path string, now time.Time, maxAge time.Duration) error {
	info, exists, err := privateDiagnosticFile(path)
	if err != nil || !exists || !diagnosticFileExpired(info, now, maxAge) {
		return err
	}
	return os.Remove(path)
}

func diagnosticFileExpired(info os.FileInfo, now time.Time, maxAge time.Duration) bool {
	return maxAge > 0 && now.Sub(info.ModTime()) >= maxAge
}

func rotateDiagnosticFile(path, archive string) error {
	if _, exists, err := privateDiagnosticFile(archive); err != nil {
		return err
	} else if exists {
		if err := os.Remove(archive); err != nil {
			return err
		}
	}
	return os.Rename(path, archive)
}

func pruneDiagnosticFiles(path, archive string, now time.Time, maxAge time.Duration, maxBytes int64) error {
	// maxBytes is supplied as the combined current/archive budget. Both
	// segments are individually capped before this final defensive check.
	if maxBytes <= 0 {
		return nil
	}
	type item struct {
		path string
		info os.FileInfo
	}
	items := make([]item, 0, 2)
	for _, candidate := range []string{path, archive} {
		info, exists, err := privateDiagnosticFile(candidate)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if diagnosticFileExpired(info, now, maxAge) {
			if err := os.Remove(candidate); err != nil {
				return err
			}
			continue
		}
		items = append(items, item{path: candidate, info: info})
	}
	var total int64
	for _, entry := range items {
		total += entry.info.Size()
	}
	if total <= maxBytes {
		return nil
	}
	sort.Slice(items, func(left, right int) bool {
		return items[left].info.ModTime().Before(items[right].info.ModTime())
	})
	for _, entry := range items {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(entry.path); err != nil {
			return err
		}
		total -= entry.info.Size()
	}
	return nil
}

func diagnosticValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(strings.ToLower(value), "token") || strings.Contains(strings.ToLower(value), "bearer") || strings.Contains(strings.ToLower(value), "credential") {
		return "redacted"
	}
	return value
}

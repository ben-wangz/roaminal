package report

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// Keep the active and rotated diagnostic segments within this total budget.
	diagnosticLogLimit        int64         = 128 << 20
	diagnosticLogSegmentLimit int64         = diagnosticLogLimit / 2
	diagnosticLogRetention    time.Duration = 48 * time.Hour
)

var diagnosticLogMu sync.Mutex

func ensurePrivateDir(path string) error {
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
		return errors.New("private diagnostic directory is unsafe")
	}
	if info.Mode().Perm() != 0700 {
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0700 {
			return errors.New("private diagnostic directory permissions are unsafe")
		}
	}
	return nil
}

func diagnosticLogPath(home string) string {
	return filepath.Join(home, ".roaminal", "logs", "codex-hook.log")
}

func diagnosticLogLockPath(home string) string {
	return diagnosticLogPath(home) + ".lock"
}

// LogDiagnostic is best effort. Hook diagnostics must never change Codex's
// hook result or expose credentials and terminal content.
func LogDiagnostic(home, event string, fields map[string]string) {
	diagnosticLogMu.Lock()
	defer diagnosticLogMu.Unlock()
	if strings.TrimSpace(home) == "" || strings.TrimSpace(event) == "" {
		return
	}
	root := filepath.Join(home, ".roaminal")
	logs := filepath.Join(root, "logs")
	if err := ensurePrivateDir(root); err != nil {
		return
	}
	if err := ensurePrivateDir(logs); err != nil {
		return
	}
	now := time.Now().UTC()
	record := diagnosticRecord(now, event, fields)
	_ = withDiagnosticFileLock(diagnosticLogLockPath(home), func() error {
		return appendDiagnosticRecord(logs, record, now, diagnosticLogSegmentLimit, diagnosticLogRetention)
	})
}

func diagnosticRecord(now time.Time, event string, fields map[string]string) []byte {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var record strings.Builder
	fmt.Fprintf(&record, "time=%q level=INFO event=%q", now.Format(time.RFC3339Nano), event)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		fmt.Fprintf(&record, " %s=%q", key, diagnosticValue(fields[key]))
	}
	record.WriteByte('\n')
	return []byte(record.String())
}

func appendDiagnosticRecord(logs string, record []byte, now time.Time, segmentLimit int64, maxAge time.Duration) error {
	if segmentLimit <= 0 || int64(len(record)) > segmentLimit {
		return fmt.Errorf("diagnostic record exceeds log segment limit")
	}
	path := filepath.Join(logs, "codex-hook.log")
	archive := path + ".1"
	if err := expireDiagnosticFile(archive, now, maxAge); err != nil {
		return err
	}
	current, exists, err := privateDiagnosticFile(path)
	if err != nil {
		return err
	}
	if exists && diagnosticFileExpired(current, now, maxAge) {
		if err := os.Remove(path); err != nil {
			return err
		}
		exists = false
	}
	if exists && current.Size()+int64(len(record)) > segmentLimit {
		if err := rotateDiagnosticFile(path, archive); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if err := validateDiagnosticFile(file, path); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(record); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return pruneDiagnosticFiles(path, archive, now, maxAge, segmentLimit*2)
}

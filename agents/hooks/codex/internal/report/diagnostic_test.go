package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogDiagnosticWritesBoundedPrivateRecords(t *testing.T) {
	home := t.TempDir()
	LogDiagnostic(home, "hook_delivery_failed", map[string]string{
		"sequence": "7",
		"error":    "Bearer token webhook must not be retained",
	})
	path := diagnosticLogPath(home)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `event="hook_delivery_failed"`) || !strings.Contains(string(data), `sequence="7"`) {
		t.Fatalf("unexpected diagnostic record: %s", data)
	}
	if strings.Contains(string(data), "Bearer") || strings.Contains(string(data), "webhook") || strings.Contains(string(data), "token") {
		t.Fatalf("diagnostic record retained sensitive value: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("diagnostic log mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestAppendDiagnosticRecordRotatesAndBoundsSegments(t *testing.T) {
	logs := t.TempDir()
	path := filepath.Join(logs, "codex-hook.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("old", 5)), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := appendDiagnosticRecord(logs, []byte("new\n"), now, 16, 48*time.Hour); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "new\n" || string(archive) != strings.Repeat("old", 5) {
		t.Fatalf("unexpected rotated logs: current=%q archive=%q", current, archive)
	}
	currentInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	archiveInfo, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if currentInfo.Size()+archiveInfo.Size() > 32 {
		t.Fatalf("diagnostic logs exceed combined budget: %d", currentInfo.Size()+archiveInfo.Size())
	}
}

func TestAppendDiagnosticRecordExpiresOldLogs(t *testing.T) {
	logs := t.TempDir()
	path := filepath.Join(logs, "codex-hook.log")
	archive := path + ".1"
	if err := os.WriteFile(path, []byte("old-current\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("old-archive\n"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expired := now.Add(-49 * time.Hour)
	if err := os.Chtimes(path, expired, expired); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(archive, expired, expired); err != nil {
		t.Fatal(err)
	}
	if err := appendDiagnosticRecord(logs, []byte("fresh\n"), now, 16, 48*time.Hour); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "fresh\n" {
		t.Fatalf("expired current log was retained: %q", current)
	}
	if _, err := os.Stat(archive); !os.IsNotExist(err) {
		t.Fatalf("expired archive still exists, stat error = %v", err)
	}
}

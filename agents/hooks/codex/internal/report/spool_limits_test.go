package report

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTrimSpoolKeepsTerminalEventsWhenPossible(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000000-stop.json"), []byte(`{"eventName":"Stop"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < 257; index++ {
		name := filepath.Join(dir, fmt.Sprintf("event-%03d.json", index))
		if err := os.WriteFile(name, []byte(`{"eventName":"PreToolUse"}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := trimSpool(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 256 {
		t.Fatalf("got %d spool entries, want 256", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, "00000000000000000000-stop.json")); err != nil {
		t.Fatalf("terminal event was evicted: %v", err)
	}
}

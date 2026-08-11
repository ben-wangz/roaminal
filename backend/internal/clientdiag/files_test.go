package clientdiag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileWriterUsesPrivateRotatingFilesAndIgnoresSymlinks(t *testing.T) {
	dir := t.TempDir()
	linkTarget := filepath.Join(t.TempDir(), "target.ndjson")
	linkName := filepath.Join(dir, "client-20260101T000000.000000000Z-aaaaaaaa.ndjson")
	if err := os.Symlink(linkTarget, linkName); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writer, err := newFileWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.WriteBatch([][]byte{[]byte(`{"ok":true}` + "\n")}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	foundRegular := false
	for _, entry := range entries {
		info, statErr := os.Lstat(filepath.Join(dir, entry.Name()))
		if statErr != nil {
			t.Fatal(statErr)
		}
		if strings.HasSuffix(entry.Name(), ".ndjson") && info.Mode().IsRegular() {
			foundRegular = true
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
			}
		}
	}
	if !foundRegular {
		t.Fatal("expected a regular diagnostics file")
	}
	if _, err := os.Stat(linkTarget); !os.IsNotExist(err) {
		t.Fatalf("writer followed symlink target: %v", err)
	}
}

func TestFileWriterRotatesAtBound(t *testing.T) {
	dir := t.TempDir()
	writer, err := newFileWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	line := []byte(strings.Repeat("x", MaxStoredFileBytes/2))
	line = append(line, '\n')
	if err := writer.WriteBatch([][]byte{line, line, line}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	regular := 0
	for _, entry := range entries {
		info, statErr := os.Lstat(filepath.Join(dir, entry.Name()))
		if statErr == nil && info.Mode().IsRegular() {
			regular++
			if info.Size() > MaxStoredFileBytes {
				t.Fatalf("file %s is larger than the bound: %d", entry.Name(), info.Size())
			}
		}
	}
	if regular < 2 {
		t.Fatalf("regular files = %d, want at least 2 after rotation", regular)
	}
}

func TestFileWriterPrunesRetentionAndFileCountWithoutTouchingUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	for index := 0; index < MaxStoredFiles+2; index++ {
		name := filepath.Join(dir, fmt.Sprintf("client-20260101T000000.%09dZ-%08x.ndjson", index, index+1))
		if err := os.WriteFile(name, []byte("old\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(name, time.Now(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(dir, "client-20260101T000001.000000000Z-ffffffff.ndjson")
	if err := os.WriteFile(old, []byte("expired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, time.Now().Add(-Retention-time.Hour), time.Now().Add(-Retention-time.Hour)); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "operator-notes.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer, err := newFileWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	managed := 0
	for _, entry := range entries {
		if managedFilePattern.MatchString(entry.Name()) {
			managed++
		}
	}
	if managed > MaxStoredFiles {
		t.Fatalf("managed files=%d, want at most %d", managed, MaxStoredFiles)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

package filesystem

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestRemoteScriptsUseNulProtocolAndBoundedContent(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "中文 file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	directory := runFilesystemScript(t, directoryScript, root, ".")
	entries, err := parseDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	foundName := false
	for _, entry := range entries {
		foundName = foundName || entry.Name == "中文 file.txt"
	}
	if len(entries) != 3 || !foundName {
		t.Fatalf("unexpected directory entries: %#v", entries)
	}
	stat := runFilesystemScript(t, statScript, root, "中文 file.txt")
	statEntries, err := parseDirectory(stat)
	if err != nil || len(statEntries) != 1 || statEntries[0].Type != "file" {
		t.Fatalf("unexpected stat: %v %#v", err, statEntries)
	}
	if statEntries[0].MIMEType != "" {
		// The development host does not necessarily have file(1) installed;
		// when it is available, the stat frame must carry a valid detected type.
		if statEntries[0].MIMEType != "text/plain" {
			t.Fatalf("unexpected detected MIME type: %q", statEntries[0].MIMEType)
		}
	}
	content := runFilesystemScript(t, contentScript, root, "中文 file.txt", "1", "3")
	if string(content) != "ell" {
		t.Fatalf("content range = %q, want ell", content)
	}
	targetOutput := runFilesystemScript(t, uploadTargetScript, root, ".")
	targetFields := strings.Split(string(targetOutput), "\x00")
	if len(targetFields) != 3 || targetFields[0] != uploadTargetMarker || targetFields[1] != root {
		t.Fatalf("unexpected upload target: %#v", targetFields)
	}
	conflictOutput := runFilesystemScript(t, uploadConflictScript, root, ".", "missing.txt")
	conflictFields := strings.Split(string(conflictOutput), "\x00")
	if len(conflictFields) != 3 || conflictFields[0] != uploadConflictMarker || conflictFields[1] != uploadConflictEnd {
		t.Fatalf("unexpected conflict output: %#v", conflictFields)
	}
	if output := runFilesystemScript(t, remoteMkdirScript, root, ".", "nested/assets"); len(output) != 0 {
		t.Fatalf("mkdir helper wrote output: %q", output)
	}
	if info, err := os.Stat(filepath.Join(root, "nested", "assets")); err != nil || !info.IsDir() {
		t.Fatalf("mkdir helper did not create directory: %v", err)
	}
}

func TestParseDirectoryAcceptsDetectedMIMEField(t *testing.T) {
	data := []byte(directoryBegin + "\x00README\x00file\x0042\x001700000000\x00644\x00false\x00text/plain\x00\x00" + directoryEnd + "\x00")
	entries, err := parseDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "README" || entries[0].MIMEType != "text/plain" {
		t.Fatalf("unexpected detected entry: %#v", entries)
	}
}

func TestCreateUploadRequiresManifestParts(t *testing.T) {
	alias := "fixture"
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
		switch {
		case command.Script == configuredRootScript:
			return rootOutput("/workspace"), nil
		case command.Script == uploadTargetScript:
			return []byte(uploadTargetMarker + "\x00/workspace\x00"), nil
		default:
			return nil, errors.New("unexpected upload command")
		}
	}
	service := New(fake, nil)
	root, err := service.Root(context.Background(), "instance")
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.CreateUpload(context.Background(), "instance", UploadManifest{
		RootRevision: root.Revision,
		TargetPath:   ".",
		Files:        []UploadManifestFile{{Part: "file-0", RelativePath: "existing.txt", Size: 4}},
	}, nil)
	if !errors.Is(err, ErrContentUnavailable) || status.UploadID != "" {
		t.Fatalf("expected missing staged part to be rejected, status=%#v err=%v", status, err)
	}
}

func TestCreateUploadRejectsUnsafeManifestBeforeStaging(t *testing.T) {
	alias := "fixture"
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
		return rootOutput("/workspace"), nil
	}
	service := New(fake, nil)
	root, err := service.Root(context.Background(), "instance")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateUpload(context.Background(), "instance", UploadManifest{
		RootRevision: root.Revision,
		TargetPath:   "../outside",
		Files:        []UploadManifestFile{{Part: "file-0", RelativePath: "file.txt", Size: 0}},
	}, nil)
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("expected outside-root error, got %v", err)
	}
}

func runFilesystemScript(t *testing.T, script string, args ...string) []byte {
	t.Helper()
	command := exec.Command("sh", "-s", "--")
	command.Args = append(command.Args, args...)
	command.Stdin = strings.NewReader(script)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	return output
}

func rootOutput(value string) []byte { return []byte(rootBeginMarker + "\x00" + value + "\x00") }

func int64Pointer(value int64) *int64 { return &value }

func directoryOutput(entries ...rawEntry) []byte {
	return framedDirectoryOutput(false, entries...)
}

func detectedDirectoryOutput(entries ...rawEntry) []byte {
	return framedDirectoryOutput(true, entries...)
}

func framedDirectoryOutput(includeMIME bool, entries ...rawEntry) []byte {
	var output bytes.Buffer
	output.WriteString(directoryBegin)
	output.WriteByte(0)
	for _, entry := range entries {
		fields := []string{entry.Name, entry.Type, "-", "-", "0", "false", ""}
		if entry.Size != nil {
			fields[2] = string(rune('0' + *entry.Size))
		}
		if includeMIME {
			mimeType := entry.MIMEType
			if mimeType == "" {
				mimeType = "-"
			}
			fields = append(fields[:6], mimeType, "")
		}
		for _, field := range fields {
			output.WriteString(field)
			output.WriteByte(0)
		}
	}
	output.WriteString(directoryEnd)
	output.WriteByte(0)
	return output.Bytes()
}

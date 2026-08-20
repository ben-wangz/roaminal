package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
)

type fakeExecutor struct {
	summary connection.Summary
	run     func(connection.RemoteCommand) ([]byte, error)
	calls   []connection.RemoteCommand
}

func (f *fakeExecutor) Summaries() []connection.Summary { return []connection.Summary{f.summary} }

func (f *fakeExecutor) RunRemote(_ context.Context, _ string, command connection.RemoteCommand) (connection.RemoteResult, error) {
	f.calls = append(f.calls, command)
	output, err := f.run(command)
	return connection.RemoteResult{Output: output}, err
}

func (f *fakeExecutor) OpenRemote(_ context.Context, _ string, _ connection.RemoteCommand) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeExecutor) RemoteTransferInfo(_ string) (connection.RemoteTransferInfo, error) {
	return connection.RemoteTransferInfo{Alias: "fixture", ControlPath: "/tmp/fixture", SSHPath: "ssh"}, nil
}

func TestRootRetriesTmuxProbe(t *testing.T) {
	alias := "fixture"
	failures := 0
	fake := &fakeExecutor{summary: connection.Summary{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias, TmuxEnabled: true, TmuxSessionName: "roaminal"}}
	fake.run = func(command connection.RemoteCommand) ([]byte, error) {
		if strings.Contains(command.Script, "tmux has-session") {
			failures++
			if failures == 1 {
				return nil, errors.New("tmux not ready")
			}
			return rootOutput("/workspace"), nil
		}
		return nil, errors.New("configured root should not run")
	}
	service := New(fake, nil)
	root, err := service.Root(context.Background(), "instance")
	if err != nil {
		t.Fatal(err)
	}
	if root.Source != "tmux" || root.Status != "current" || failures != 2 {
		t.Fatalf("root=%#v tmux attempts=%d", root, failures)
	}
}

func TestRootFailureUsesConfiguredFallbackAndRetriesAfterCache(t *testing.T) {
	alias := "fixture"
	tmuxCalls := 0
	configuredCalls := 0
	fake := &fakeExecutor{summary: connection.Summary{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias, TmuxEnabled: true, TmuxSessionName: "roaminal"}}
	fake.run = func(command connection.RemoteCommand) ([]byte, error) {
		if strings.Contains(command.Script, "tmux has-session") {
			tmuxCalls++
			return nil, errors.New("tmux unavailable")
		}
		configuredCalls++
		return rootOutput("/home/coder"), nil
	}
	service := New(fake, nil)
	first, err := service.Root(context.Background(), "instance")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Root(context.Background(), "instance")
	if err != nil {
		t.Fatal(err)
	}
	if first.Source != "configured" || first.Status != "fallback" || second.Revision != first.Revision {
		t.Fatalf("unexpected fallback roots: %#v %#v", first, second)
	}
	if tmuxCalls != 2 || configuredCalls != 2 {
		t.Fatalf("failure cache did not suppress one probe: tmux=%d configured=%d", tmuxCalls, configuredCalls)
	}
	service.now = func() time.Time { return time.Now().Add(2 * time.Second) }
	if _, err := service.Root(context.Background(), "instance"); err != nil {
		t.Fatal(err)
	}
	if tmuxCalls != 4 {
		t.Fatalf("expired failure cache did not retry tmux: %d", tmuxCalls)
	}
}

func TestDirectoryProtocolAndStableOrdering(t *testing.T) {
	alias := "fixture"
	fake := &fakeExecutor{summary: connection.Summary{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command connection.RemoteCommand) ([]byte, error) {
		if command.Script == configuredRootScript {
			return rootOutput("/workspace"), nil
		}
		return directoryOutput(
			rawEntry{Name: "z.txt", Type: "file", Size: int64Pointer(2), Mode: 420},
			rawEntry{Name: "src", Type: "directory", Mode: 493},
			rawEntry{Name: "中文.md", Type: "file", Mode: 420},
		), nil
	}
	service := New(fake, nil)
	result, err := service.Entries(context.Background(), "instance", ".", "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 || result.Entries[0].Name != "src" || result.NextCursor == nil {
		t.Fatalf("unexpected first page: %#v", result)
	}
	next, err := service.Entries(context.Background(), "instance", ".", result.RootRevision, *result.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Entries) != 1 || next.Entries[0].Name != "中文.md" || next.NextCursor != nil {
		t.Fatalf("unexpected second page: %#v", next)
	}
}

func TestEntriesRejectsRootRevisionChange(t *testing.T) {
	alias := "fixture"
	rootPath := "/workspace"
	fake := &fakeExecutor{summary: connection.Summary{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command connection.RemoteCommand) ([]byte, error) {
		if command.Script == configuredRootScript {
			result := rootOutput(rootPath)
			rootPath = "/other"
			return result, nil
		}
		return directoryOutput(), nil
	}
	service := New(fake, nil)
	root, err := service.Root(context.Background(), "instance")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Entries(context.Background(), "instance", ".", root.Revision, "", 0)
	var changed *RootChangedError
	if !errors.As(err, &changed) || changed.Root.AbsolutePath != "/other" {
		t.Fatalf("expected root change, got %v", err)
	}
}

func TestValidateRelativePath(t *testing.T) {
	for _, value := range []string{"", ".", "src", "src//main.go", "中文/README.md"} {
		if _, err := ValidateRelativePath(value); err != nil {
			t.Errorf("path %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"/etc", "../etc", "src/../etc", "src\\main.go", "src\x00main"} {
		if _, err := ValidateRelativePath(value); err == nil {
			t.Errorf("path %q should be rejected", value)
		}
	}
}

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

func TestCreateUploadRequiresManifestParts(t *testing.T) {
	alias := "fixture"
	fake := &fakeExecutor{summary: connection.Summary{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command connection.RemoteCommand) ([]byte, error) {
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
	fake := &fakeExecutor{summary: connection.Summary{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command connection.RemoteCommand) ([]byte, error) {
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
	var output bytes.Buffer
	output.WriteString(directoryBegin)
	output.WriteByte(0)
	for _, entry := range entries {
		fields := []string{entry.Name, entry.Type, "-", "-", "0", "false", ""}
		if entry.Size != nil {
			fields[2] = string(rune('0' + *entry.Size))
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

package filesystem

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type fakeExecutor struct {
	summary ports.ConnectionInstanceView
	run     func(ports.RemoteCommand) ([]byte, error)
	calls   []ports.RemoteCommand
}

func (f *fakeExecutor) ConnectionInstance(_ string) (ports.ConnectionInstanceView, error) {
	return f.summary, nil
}

func (f *fakeExecutor) RunRemote(_ context.Context, _ string, command ports.RemoteCommand) (ports.RemoteResult, error) {
	f.calls = append(f.calls, command)
	output, err := f.run(command)
	return ports.RemoteResult{Output: output}, err
}

func (f *fakeExecutor) OpenRemote(_ context.Context, _ string, _ ports.RemoteCommand) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeExecutor) RemoteTransferInfo(_ string) (ports.RemoteTransferInfo, error) {
	return ports.RemoteTransferInfo{Alias: "fixture", ControlPath: "/tmp/fixture", SSHPath: "ssh"}, nil
}

func (f *fakeExecutor) AcquireRemoteTransfer(_ context.Context, _ string) (ports.RemoteTransferLease, error) {
	return fakeTransferLease{info: ports.RemoteTransferInfo{Alias: "fixture", ControlPath: "/tmp/fixture", SSHPath: "ssh"}}, nil
}

type fakeTransferLease struct {
	info ports.RemoteTransferInfo
}

func (l fakeTransferLease) Info() ports.RemoteTransferInfo { return l.info }
func (l fakeTransferLease) Close()                         {}

func TestRootRetriesTmuxProbe(t *testing.T) {
	alias := "fixture"
	failures := 0
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias, TmuxEnabled: true, TmuxSessionName: "roaminal"}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
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
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias, TmuxEnabled: true, TmuxSessionName: "roaminal"}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
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
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
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

func TestStatPreservesDetectedMIMEType(t *testing.T) {
	alias := "fixture"
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
		if command.Script == configuredRootScript {
			return rootOutput("/workspace"), nil
		}
		return detectedDirectoryOutput(rawEntry{Name: "README", Type: "file", MIMEType: "text/plain", Size: int64Pointer(4)}), nil
	}
	entry, _, err := New(fake, nil).Stat(context.Background(), "instance", "README", "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.MIMEType != "text/plain" {
		t.Fatalf("detected MIME type = %q, want text/plain", entry.MIMEType)
	}
}

func TestEntriesRejectsRootRevisionChange(t *testing.T) {
	alias := "fixture"
	rootPath := "/workspace"
	fake := &fakeExecutor{summary: ports.ConnectionInstanceView{ID: "instance", Type: "ssh", Lifecycle: "live", SourceHostAlias: &alias}}
	fake.run = func(command ports.RemoteCommand) ([]byte, error) {
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
	for _, value := range []string{"", ".", "src", "中文/README.md", "line\nname.txt"} {
		if _, err := ValidateRelativePath(value); err != nil {
			t.Errorf("path %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"/etc", "../etc", "src/../etc", "src//main.go", "src/./main.go", "src\\main.go", "src\x00main"} {
		if _, err := ValidateRelativePath(value); err == nil {
			t.Errorf("path %q should be rejected", value)
		}
	}
}

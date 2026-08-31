package tmux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestParseInfo(t *testing.T) {
	info, err := parseInfo([]string{"roaminal", "$0", "1786613448", "%0", "/tmp/tmux-1000/default"})
	if err != nil {
		t.Fatal(err)
	}
	if info.SessionName != "roaminal" || info.SessionID != "$0" || info.PaneID != "%0" || info.SessionCreated != 1786613448 || len(info.SocketFingerprint) != 16 {
		t.Fatalf("unexpected tmux info: %+v", info)
	}
}

func TestParseInfoRejectsUnsafeValues(t *testing.T) {
	for _, parts := range [][]string{
		{"roaminal\nother", "$0", "1", "%0", "/tmp/tmux"},
		{"roaminal", "$0", "1", "%0", ""},
		{"roaminal", "$0", "-1", "%0", "/tmp/tmux"},
	} {
		if _, err := parseInfo(parts); err == nil {
			t.Fatalf("expected invalid identity for %#v", parts)
		}
	}
}

func TestRuntimeIDSeparatesTmuxRuntimes(t *testing.T) {
	base := Info{SessionName: "team", SessionID: "$1", SessionCreated: 10, PaneID: "%0", SocketFingerprint: "0123456789abcdef"}
	if RuntimeID(base) == RuntimeID(Info{SessionName: base.SessionName, SessionID: "$2", SessionCreated: base.SessionCreated, PaneID: base.PaneID, SocketFingerprint: base.SocketFingerprint}) {
		t.Fatal("different tmux sessions must not share a runtime identity")
	}
	if RuntimeID(base) == RuntimeID(Info{SessionName: base.SessionName, SessionID: base.SessionID, SessionCreated: 11, PaneID: base.PaneID, SocketFingerprint: base.SocketFingerprint}) {
		t.Fatal("recreated tmux sessions must not share a runtime identity")
	}
	if RuntimeID(base) == RuntimeID(Info{SessionName: base.SessionName, SessionID: base.SessionID, SessionCreated: base.SessionCreated, PaneID: base.PaneID, SocketFingerprint: "fedcba9876543210"}) {
		t.Fatal("different tmux sockets must not share a runtime identity")
	}
}

func TestWithRuntimeLockSerializesSameTmuxRuntime(t *testing.T) {
	home := t.TempDir()
	info := Info{SessionID: "$1", SessionCreated: 10, SocketFingerprint: "0123456789abcdef"}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- WithRuntimeLock(context.Background(), home, info, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- WithRuntimeLock(context.Background(), home, info, func() error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second operation acquired the same tmux lock too early")
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestWithRuntimeLockHonorsContextWhenLockIsHeld(t *testing.T) {
	home := t.TempDir()
	info := Info{SessionID: "$1", SessionCreated: 10, SocketFingerprint: "0123456789abcdef"}
	path := tmuxLockPath(home, info)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if err := WithRuntimeLock(ctx, home, info, func() error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WithRuntimeLock error = %v, want context deadline", err)
	}
}

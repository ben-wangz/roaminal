package terminal

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestTerminateSessionRunsExitHook(t *testing.T) {
	cwd := t.TempDir()
	manager := NewManager(config.Config{InitialCwd: cwd}, nil, nil)
	id := "11111111-1111-4111-8111-111111111111"
	session, err := manager.startCommand(persistence.ConnectionInstanceMeta{ID: id, Cwd: cwd, Cols: 80, Rows: 24}, cwd, []string{"/bin/sh", "-c", "sleep 30"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hookCalled := make(chan ExitStatus, 1)
	session.onExit = func(status ExitStatus) { hookCalled <- status }
	manager.sessions[id] = session
	manager.startLoops(session)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := manager.terminateSession(ctx, session, "exited"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-hookCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("explicit termination did not run exit hook")
	}
	if _, ok := manager.sessions[id]; ok {
		t.Fatal("terminated session remained registered")
	}
}

func TestAttachReservationIsAtomic(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	manager := NewManager(config.Config{MaxClientsPerConnectionInstance: 1}, store, nil)
	manager.sessions[id] = &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{ID: id}, clients: map[*Client]struct{}{}}
	if err := manager.ReserveAttach(id); err != nil {
		t.Fatal(err)
	}
	if err := manager.ReserveAttach(id); err != ErrClientCapacity {
		t.Fatalf("second reservation error = %v", err)
	}
	manager.ReleaseAttach(id)
	if err := manager.ReserveAttach(id); err != nil {
		t.Fatal(err)
	}
}

func TestControlOwnerRejectsNonOwnerInput(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	owner, other := newClient(), newClient()
	manager := NewManager(config.Config{}, store, nil)
	manager.sessions[id] = &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{ID: id}, clients: map[*Client]struct{}{owner: {}, other: {}}}
	if err := manager.ClaimControl(id, owner); err != nil {
		t.Fatal(err)
	}
	if err := manager.Input(id, other, "echo ignored\n"); err != ErrControlNotOwner {
		t.Fatalf("non-owner input error = %v", err)
	}
}

func TestSlowClientCarriesCloseReason(t *testing.T) {
	client := newClient()
	data := make([]byte, 16*1024)
	for index := 0; index < 300; index++ {
		if client.enqueue(data, true) == false {
			break
		}
	}
	code, reason := client.CloseReason()
	if code != 1013 || reason != "slow_client" {
		t.Fatalf("close reason = %d %q", code, reason)
	}
}

func TestDecodeUTF8PreservesPartialRune(t *testing.T) {
	text, rest := decodeUTF8([]byte{0xe2, 0x82})
	if text != "" || string(rest) != "\xe2\x82" {
		t.Fatalf("expected buffered partial rune, got %q rest=%q", text, rest)
	}
	text, rest = decodeUTF8([]byte{0xe2, 0x82, 0xac})
	if text != "€" || rest != nil {
		t.Fatalf("expected euro rune, got %q rest=%q", text, rest)
	}
	text, rest = decodeUTF8([]byte("ok\xe4\xbd"))
	if text != "ok" || string(rest) != "\xe4\xbd" {
		t.Fatalf("expected complete prefix with buffered tail, got %q rest=%q", text, rest)
	}
}

func TestPrivateMarkersAreFilteredAcrossChunks(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{ScrollbackLines: 1000}, store, nil)
	now := time.Now().UTC()
	session := &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{FormatVersion: persistence.ConnectionFormatVersion, ID: "11111111-1111-4111-8111-111111111111", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}}
	encoded := base64.StdEncoding.EncodeToString([]byte("/tmp"))
	first := "before\x1b]777;roaminal;cwd:" + encoded[:4]
	session.mu.Lock()
	if got := session.parseMarkersLocked([]byte(first)); string(got) != "before" {
		t.Fatalf("first chunk leaked marker: %q", got)
	}
	if got := session.parseMarkersLocked([]byte(encoded[4:] + "\x07after")); string(got) != "after" {
		t.Fatalf("second chunk mismatch: %q", got)
	}
	if session.meta.Cwd != "/tmp" {
		t.Fatalf("cwd marker was not decoded: %q", session.meta.Cwd)
	}
	title := "\x1b]0;terminal\x07"
	if got := session.parseMarkersLocked([]byte(title)); string(got) != title || !strings.EqualFold(session.meta.Title, "terminal") {
		t.Fatalf("title sequence was not preserved: %q title=%q", got, session.meta.Title)
	}
	session.mu.Unlock()
}

func TestSplitRuneAcrossMarkerBoundary(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{ScrollbackLines: 1000}, store, nil)
	now := time.Now().UTC()
	session := &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{FormatVersion: persistence.ConnectionFormatVersion, ID: "22222222-2222-4222-8222-222222222222", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}}
	encoded := base64.StdEncoding.EncodeToString([]byte("/tmp"))
	marker := "\x1b]777;roaminal;cwd:" + encoded + "\x07"
	// The shell integration emits a marker between two commands whose output
	// splits one rune: 你 = e4 bd a0, 好 = e5 a5 bd.
	session.mu.Lock()
	defer session.mu.Unlock()
	var out strings.Builder
	for _, chunk := range [][]byte{[]byte("\xe4\xbd" + marker), []byte("\xa0\xe5\xa5\xbd ok")} {
		stripped := session.parseMarkersLocked(chunk)
		session.pending = append(session.pending, stripped...)
		text, rest := decodeUTF8(session.pending)
		session.pending = rest
		out.WriteString(text)
	}
	if got := out.String(); got != "你好 ok" {
		t.Fatalf("split rune across marker corrupted: %q", got)
	}
}

func TestSummariesHaveDeterministicOrder(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{}, store, nil)
	base := time.Now().UTC()
	ids := []string{
		"33333333-3333-4333-8333-333333333333",
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	}
	manager.sessions[ids[0]] = &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{ID: ids[0], InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: base, UpdatedAt: base}}
	manager.sessions[ids[1]] = &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{ID: ids[1], InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: base, UpdatedAt: base}}
	manager.sessions[ids[2]] = &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{ID: ids[2], InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second)}}
	want := []string{ids[1], ids[0], ids[2]}
	for attempt := 0; attempt < 20; attempt++ {
		got := manager.Summaries()
		for index, summary := range got {
			if summary.ID != want[index] {
				t.Fatalf("attempt %d summary order %v, want %v", attempt, got, want)
			}
		}
	}
}

func TestSetTitlePersistsOverrideAndAutomaticReset(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{}, store, nil)
	now := time.Now().UTC()
	id := "11111111-1111-4111-8111-111111111111"
	manager.sessions[id] = &Session{manager: manager, meta: persistence.ConnectionInstanceMeta{FormatVersion: persistence.ConnectionFormatVersion, ID: id, AutomaticTitle: "shell", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}, clients: map[*Client]struct{}{}}
	custom := "custom title"
	result, err := manager.SetTitle(id, &custom)
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != custom || result.TitleMode != "custom" {
		t.Fatalf("unexpected custom title result: %+v", result)
	}
	loaded, err := store.LoadConnectionInstance(id)
	if err != nil || loaded.TitleOverride == nil || *loaded.TitleOverride != custom {
		t.Fatalf("custom title was not persisted: %+v %v", loaded, err)
	}
	result, err = manager.SetTitle(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "shell" || result.TitleMode != "automatic" {
		t.Fatalf("unexpected automatic title result: %+v", result)
	}
}

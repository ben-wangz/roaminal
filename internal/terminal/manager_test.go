package terminal

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/internal/config"
	"github.com/ben-wangz/roaminal/internal/persistence"
)

func TestAttachReservationIsAtomic(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "11111111-1111-4111-8111-111111111111"
	manager := NewManager(config.Config{MaxClientsPerSession: 1}, store, nil)
	manager.sessions[id] = &Session{manager: manager, meta: persistence.SessionMeta{ID: id}, clients: map[*Client]struct{}{}}
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
	manager.sessions[id] = &Session{manager: manager, meta: persistence.SessionMeta{ID: id}, clients: map[*Client]struct{}{owner: {}, other: {}}}
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
	text, complete := decodeUTF8([]byte{0xe2, 0x82})
	if complete || text != "" {
		t.Fatalf("expected partial rune, got %q complete=%v", text, complete)
	}
	text, complete = decodeUTF8([]byte{0xe2, 0x82, 0xac})
	if !complete || text != "€" {
		t.Fatalf("expected euro rune, got %q complete=%v", text, complete)
	}
}

func TestPrivateMarkersAreFilteredAcrossChunks(t *testing.T) {
	store, err := persistence.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(config.Config{ScrollbackLines: 1000}, store, nil)
	now := time.Now().UTC()
	session := &Session{manager: manager, meta: persistence.SessionMeta{FormatVersion: persistence.FormatVersion, ID: "11111111-1111-4111-8111-111111111111", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}}
	encoded := base64.StdEncoding.EncodeToString([]byte("/tmp"))
	first := "before\x1b]777;roaminal;cwd:" + encoded[:4]
	session.mu.Lock()
	if got := session.parseMarkersLocked(first); got != "before" {
		t.Fatalf("first chunk leaked marker: %q", got)
	}
	if got := session.parseMarkersLocked(encoded[4:] + "\x07after"); got != "after" {
		t.Fatalf("second chunk mismatch: %q", got)
	}
	if session.meta.Cwd != "/tmp" {
		t.Fatalf("cwd marker was not decoded: %q", session.meta.Cwd)
	}
	title := "\x1b]0;terminal\x07"
	if got := session.parseMarkersLocked(title); got != title || !strings.EqualFold(session.meta.Title, "terminal") {
		t.Fatalf("title sequence was not preserved: %q title=%q", got, session.meta.Title)
	}
	session.mu.Unlock()
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
	manager.sessions[ids[0]] = &Session{manager: manager, meta: persistence.SessionMeta{ID: ids[0], InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: base, UpdatedAt: base}}
	manager.sessions[ids[1]] = &Session{manager: manager, meta: persistence.SessionMeta{ID: ids[1], InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: base, UpdatedAt: base}}
	manager.sessions[ids[2]] = &Session{manager: manager, meta: persistence.SessionMeta{ID: ids[2], InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second)}}
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
	manager.sessions[id] = &Session{manager: manager, meta: persistence.SessionMeta{FormatVersion: persistence.SessionFormatVersion, ID: id, AutomaticTitle: "shell", InitialCwd: "/workspace", Cwd: "/workspace", Cols: 80, Rows: 24, CreatedAt: now, UpdatedAt: now}, clients: map[*Client]struct{}{}}
	custom := "custom title"
	result, err := manager.SetTitle(id, &custom)
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != custom || result.TitleMode != "custom" {
		t.Fatalf("unexpected custom title result: %+v", result)
	}
	loaded, err := store.LoadSession(id)
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

package terminal

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/internal/config"
	"github.com/ben-wangz/roaminal/internal/persistence"
)

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

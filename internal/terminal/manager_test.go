package terminal

import "testing"

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

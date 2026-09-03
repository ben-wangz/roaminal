package server

import (
	"errors"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/filesystem"
)

func TestContentRange(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		start   int64
		length  int64
		partial bool
		err     error
	}{
		{name: "full", header: "", start: 0, length: 10},
		{name: "bounded", header: "bytes=2-5", start: 2, length: 4, partial: true},
		{name: "open ended", header: "bytes=7-", start: 7, length: 3, partial: true},
		{name: "suffix", header: "bytes=-4", start: 6, length: 4, partial: true},
		{name: "invalid unit", header: "items=0-1", err: filesystem.ErrInvalidRange},
		{name: "invalid start", header: "bytes=10-", err: filesystem.ErrInvalidRange},
		{name: "multiple ranges", header: "bytes=0-1,3-4", err: filesystem.ErrInvalidRange},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, length, partial, err := contentRange(test.header, 10)
			if !errors.Is(err, test.err) || start != test.start || length != test.length || partial != test.partial {
				t.Fatalf("contentRange(%q) = %d, %d, %t, %v", test.header, start, length, partial, err)
			}
		})
	}
}

func TestMimeTypeForEntryPrefersExtensionThenDetectedType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		detected string
		want     string
	}{
		{name: "detected text for unknown extension", filename: "README", detected: "text/plain", want: "text/plain"},
		{name: "detected type is normalized", filename: "README", detected: "TEXT/X-SHELLSCRIPT", want: "text/x-shellscript"},
		{name: "supported extension wins over detected type", filename: "notes.json", detected: "text/plain", want: "application/json"},
		{name: "empty detection falls back to extension", filename: "notes.json", detected: "-", want: "application/json"},
		{name: "unknown detection falls back to extension", filename: "notes.json", detected: "", want: "application/json"},
		{name: "generic binary extension allows detected text", filename: "notes.bin", detected: "text/plain", want: "text/plain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mimeTypeForEntry(test.filename, "file", test.detected); got != test.want {
				t.Fatalf("mimeTypeForEntry(%q, file, %q) = %q, want %q", test.filename, test.detected, got, test.want)
			}
		})
	}
	if got := mimeTypeForEntry("notes.txt", "file", "text/plain\r\nX-Injected: true"); got != "text/plain; charset=utf-8" {
		t.Fatalf("invalid detected MIME must fall back to extension, got %q", got)
	}
}

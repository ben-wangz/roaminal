package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	data, err := frame(map[string]any{"op": "write", "sequence": "1"}, []byte("ansi"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		t.Fatal(err)
	}
	var op string
	_ = json.Unmarshal(got.Header["op"], &op)
	if op != "write" || string(got.Payload) != "ansi" {
		t.Fatalf("unexpected frame: %+v %q", got.Header, got.Payload)
	}
}

func TestFrameRejectsOversizedHeader(t *testing.T) {
	_, err := frame(map[string]any{"payload": strings.Repeat("x", HeaderLimit)}, nil)
	if err == nil {
		t.Fatal("expected oversized header to be rejected")
	}
}

func TestWriterLimitsMatchContract(t *testing.T) {
	if WriterQueueLimit != 16*1024*1024 || WriterStallLimit != 10*time.Second {
		t.Fatalf("unexpected writer limits: queue=%d stall=%s", WriterQueueLimit, WriterStallLimit)
	}
}

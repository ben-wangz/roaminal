package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
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

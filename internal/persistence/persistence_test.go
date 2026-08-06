package persistence

import (
	"os"
	"testing"
)

func TestSnapshotRoundTripAndCorruptionIsolation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("\x1b[2Jhello\n")
	if err := store.SaveSnapshot("11111111-1111-4111-8111-111111111111", SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 1000, ThroughSequence: "4"}, payload); err != nil {
		t.Fatal(err)
	}
	header, got, err := store.LoadSnapshot("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if header.ByteLength != len(payload) || string(got) != string(payload) {
		t.Fatalf("round trip mismatch: %+v %q", header, got)
	}
	if err := os.WriteFile(store.SnapshotPath("11111111-1111-4111-8111-111111111111"), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadSnapshot("11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("expected corruption error")
	}
	entries, _ := os.ReadDir(store.SessionsDir)
	found := false
	for _, entry := range entries {
		if len(entry.Name()) > len(".snapshot.corrupt.") && entry.Name() != "11111111-1111-4111-8111-111111111111.snapshot" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected quarantined snapshot")
	}
}

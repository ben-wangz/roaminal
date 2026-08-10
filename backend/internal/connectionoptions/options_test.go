package connectionoptions

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionNameValidation(t *testing.T) {
	valid := []string{"t", "Prod_1", "a-" + string(make([]byte, 62))}
	if !ValidSessionName(valid[0]) || !ValidSessionName(valid[1]) {
		t.Fatal("expected valid session names")
	}
	for _, value := range []string{"", "1session", "a b", "a/", "a" + string(make([]byte, 64))} {
		if ValidSessionName(value) {
			t.Fatalf("session name %q should be invalid", value)
		}
	}
}

func TestStoreRoundTripAndAliasCleanup(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Save(map[string]Tmux{"alpha": {Enabled: true, SessionName: "t"}, "stale": {Enabled: true, SessionName: "old"}}); err != nil {
		t.Fatal(err)
	}
	collection, err := store.Load(map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	if collection.Options["alpha"].SessionName != "t" || len(collection.Options) != 1 {
		t.Fatalf("unexpected options: %#v", collection.Options)
	}
	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatal("reconciliation should retain the non-empty file")
	}
	if err := store.Save(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty options should be removed, got %v", err)
	}
}

func TestStoreRejectsUnknownAndDuplicateFields(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	for name, data := range map[string]string{
		"unknown":   "formatVersion: 1\nconnections: {}\nextra: true\n",
		"duplicate": "formatVersion: 1\nconnections:\n  alpha:\n    tmux:\n      enabled: true\n      enabled: false\n      sessionName: t\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, "ssh-connection-options.yaml"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(map[string]bool{"alpha": true}); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("%s: expected invalid format, got %v", name, err)
		}
	}
}

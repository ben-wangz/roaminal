package connectionoptions

import (
	"bytes"
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

func TestPwdValidation(t *testing.T) {
	for _, value := range []string{"$HOME", "~", "$HOME/projects", "~/projects", "/srv/work"} {
		if !ValidPwd(value) {
			t.Fatalf("pwd %q should be valid", value)
		}
	}
	for _, value := range []string{"", "relative", "../work", "$HOME\nwork", "/tmp\x00work", "/tmp\twork"} {
		if ValidPwd(value) {
			t.Fatalf("pwd %q should be invalid", value)
		}
	}
}

func TestStoreLoadIsSideEffectFreeAndAliasCleanupIsExplicit(t *testing.T) {
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
	data, err := os.ReadFile(store.Path())
	if err != nil || !bytes.Contains(data, []byte("stale")) {
		t.Fatalf("Load must not reconcile stale entries: %v", err)
	}
	if err := store.RemoveAlias("stale"); err != nil {
		t.Fatal(err)
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

func TestStoreLoadsV1WithDefaultPwd(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	data := "formatVersion: 1\nconnections:\n  alpha:\n    tmux:\n      enabled: true\n      sessionName: t\n"
	if err := os.WriteFile(store.Path(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	collection, err := store.Load(map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	option := collection.Options["alpha"]
	if !option.Enabled || option.SessionName != "t" || option.Pwd != DefaultPwd {
		t.Fatalf("unexpected v1 option: %#v", option)
	}
}

func TestStoreRoundTripsFilesystemOnlyOption(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Save(map[string]Tmux{"alpha": {Pwd: "~/workspace"}}); err != nil {
		t.Fatal(err)
	}
	collection, err := store.Load(map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	option := collection.Options["alpha"]
	if option.Enabled || option.Pwd != "~/workspace" {
		t.Fatalf("unexpected filesystem option: %#v", option)
	}
}

func TestStoreMovesAndCopiesAliasesExplicitly(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Save(map[string]Tmux{"alpha": {Enabled: true, SessionName: "tmux-a", Pwd: "~/work"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.MoveAlias("alpha", "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := store.CopyAlias("renamed", "copy"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Options["alpha"]; ok {
		t.Fatal("old alias survived explicit move")
	}
	if loaded.Options["renamed"].SessionName != "tmux-a" || loaded.Options["copy"].Pwd != "~/work" {
		t.Fatalf("explicit alias operations lost settings: %#v", loaded.Options)
	}
}

func TestExplicitAliasDeletePreservesOtherRawEntries(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	data := "formatVersion: 2\nconnections:\n  empty:\n    tmux:\n      enabled: false\n  target:\n    filesystem:\n      pwd: ~/target\n"
	if err := os.WriteFile(store.Path(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveAlias("target"); err != nil {
		t.Fatal(err)
	}
	remaining, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(remaining, []byte("empty")) || bytes.Contains(remaining, []byte("target")) {
		t.Fatalf("explicit delete changed unrelated raw entries: %s", remaining)
	}
}

func TestUpdateAliasPreservesUnrelatedRawEntries(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	data := "formatVersion: 2\nconnections:\n  empty:\n    tmux:\n      enabled: false\n  target:\n    filesystem:\n      pwd: ~/target\n"
	if err := os.WriteFile(store.Path(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAlias("target", Tmux{Pwd: "~/changed"}, true); err != nil {
		t.Fatal(err)
	}
	remaining, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(remaining, []byte("empty")) || !bytes.Contains(remaining, []byte("~/changed")) || bytes.Contains(remaining, []byte("~/target")) {
		t.Fatalf("single-alias update changed unrelated entries: %s", remaining)
	}
}

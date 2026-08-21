package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeIsIdempotentAndPreservesCustomHook(t *testing.T) {
	input := []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"custom"}]}]}}`)
	first, err := Merge(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Merge(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("merge is not deterministic")
	}
	var root map[string]any
	if err := json.Unmarshal(first, &root); err != nil {
		t.Fatal(err)
	}
	hooks := root["hooks"].(map[string]any)
	groups := hooks["SessionStart"].([]any)
	count := 0
	for _, group := range groups {
		for _, value := range group.(map[string]any)["hooks"].([]any) {
			if value.(map[string]any)["command"] == Command {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one canonical hook, got %d", count)
	}
}

func TestMergeRejectsNonObjectHooks(t *testing.T) {
	for _, input := range []string{"null", `{"hooks":null}`, `{"hooks":[]}`} {
		if _, err := Merge([]byte(input)); err == nil {
			t.Fatalf("expected invalid hooks config for %s", input)
		}
	}
}

func TestInstallHooksCreatesOneBackup(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, ".codex")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "hooks.json")
	original := []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"custom"}]}]}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := InstallHooks(home); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".roaminal.bak")
	if err != nil || string(backup) != string(original) {
		t.Fatalf("backup mismatch: err=%v", err)
	}
	if err := InstallHooks(home); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path + ".roaminal.bak"); err != nil || string(got) != string(original) {
		t.Fatalf("backup changed: err=%v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected hooks permissions: %v", err)
	}
}

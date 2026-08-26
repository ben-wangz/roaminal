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

func TestMergeRemovesAsyncFromExistingCanonicalHooks(t *testing.T) {
	input, err := json.Marshal(map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{"type": "command", "command": Command, "timeout": 5, "async": true},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Merge(input)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	events, ok := root["hooks"].(map[string]any)
	if !ok {
		t.Fatal("merged hooks are not an object")
	}
	canonicalCount := 0
	for event, rawGroups := range events {
		groups, ok := rawGroups.([]any)
		if !ok {
			t.Fatalf("%s hooks are not an array", event)
		}
		for _, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				t.Fatalf("%s hook group is not an object", event)
			}
			handlers, ok := group["hooks"].([]any)
			if !ok {
				t.Fatalf("%s hook handlers are not an array", event)
			}
			for _, rawHandler := range handlers {
				handler, ok := rawHandler.(map[string]any)
				if !ok || handler["command"] != Command {
					continue
				}
				canonicalCount++
				if _, exists := handler["async"]; exists {
					t.Fatalf("%s canonical hook still contains async", event)
				}
			}
		}
	}
	if canonicalCount != 9 {
		t.Fatalf("expected one synchronous canonical hook for each event, got %d", canonicalCount)
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

func TestInstallHooksRepairsPermissions(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, ".codex")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "hooks.json")
	if err := os.WriteFile(path, []byte(`{"hooks":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := InstallHooks(home); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("got mode %o, want 0600", info.Mode().Perm())
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0755 {
		t.Fatalf("InstallHooks unexpectedly changed directory mode to %o", directoryInfo.Mode().Perm())
	}
}

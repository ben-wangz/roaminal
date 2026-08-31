package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStoreRejectsUnknownFieldsAndWrongSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agent-endpoints.json")
	if err := os.WriteFile(path, []byte(`{"formatVersion":1,"endpoints":{},"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := OpenStore(root)
	if store.Available() || store.Err() == nil {
		t.Fatalf("store accepted invalid file: %v", store.Err())
	}
}

func TestStoreUpdatePersistsAtomicallyAndReturnsClones(t *testing.T) {
	root := t.TempDir()
	store := OpenStore(root)
	if err := store.Update("endpoint", func(record *EndpointRecord) error {
		record.Aliases = []string{"a"}
		record.Targets = map[string]TargetState{"tmux": {SessionName: "roaminal"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	record, ok := store.Get("endpoint")
	if !ok {
		t.Fatal("updated record missing")
	}
	record.Aliases[0] = "mutated"
	record.Targets["tmux"] = TargetState{}
	stored, ok := store.Get("endpoint")
	if !ok || stored.Aliases[0] != "a" || stored.Targets["tmux"].SessionName != "roaminal" {
		t.Fatalf("store returned mutable state: %#v", stored)
	}
	reopened := OpenStore(root)
	if !reopened.Available() {
		t.Fatalf("reopen failed: %v", reopened.Err())
	}
	if persisted, ok := reopened.Get("endpoint"); !ok || len(persisted.Aliases) != 1 || persisted.Aliases[0] != "a" {
		t.Fatalf("persisted record = %#v, %v", persisted, ok)
	}
	info, err := os.Stat(filepath.Join(root, "agent-endpoints.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("agent store permissions = %o", info.Mode().Perm())
	}
}

func TestStoreOpensLegacyWebhookMetadataForMigration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agent-endpoints.json")
	data := []byte(`{"formatVersion":1,"endpoints":{"legacy":{"user":"coder","host":"host.test","port":22,"activeTokenHash":"legacy-token","webhookOrigin":"https://legacy.invalid"}}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := OpenStore(root)
	if err := store.Err(); err != nil {
		t.Fatalf("legacy store should remain readable: %v", err)
	}
	record, ok := store.Get("legacy")
	if !ok || record.WebhookOrigin != "https://legacy.invalid" {
		t.Fatalf("legacy metadata was not decoded: ok=%v record=%+v", ok, record)
	}
}

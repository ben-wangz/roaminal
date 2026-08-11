package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func TestParseKeepsUnknownAndIgnoresInclude(t *testing.T) {
	data := []byte("# keep\nInclude conf.d/*\nHost prod-api\n  HostName 10.0.0.1 # comment\n  User deploy\n  IdentityFile ~/.ssh/id_ed25519\n  StrictHostKeyChecking no\nHost *.example\n  User wildcard\n")
	doc := Parse(data, sshfs.Capability{Status: "available", Readable: true})
	defs := doc.Definitions(map[string]bool{"id_ed25519": true})
	if len(defs) != 1 || defs[0].HostAlias != "prod-api" || defs[0].HostName == nil || *defs[0].HostName != "10.0.0.1" {
		t.Fatalf("definitions=%+v", defs)
	}
	if len(doc.Warnings) == 0 || doc.Warnings[0].Class != "include_ignored" {
		t.Fatalf("warnings=%+v", doc.Warnings)
	}
}

func TestRepositoryUpdateIsLosslessAndUsesETag(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := sshfs.OpenAt(filepath.Join(dir, ".ssh"))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	original := "Host prod\n  HostName old.example # keep\n  User deploy\nUnknownThing value\n"
	if err := os.WriteFile(filepath.Join(dir, ".ssh", "config"), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := New(root)
	doc, etag, _, err := repo.Read(map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	value := "new.example"
	collection, err := repo.Update(etag, nil, "prod", Edit{HostAlias: "prod", HostName: &value})
	if err != nil {
		t.Fatal(err)
	}
	if collection.ConfigSource.Warnings == nil || collection.ConfigSource.Blockers == nil {
		t.Fatalf("source arrays must be non-nil: %+v", collection.ConfigSource)
	}
	if collection.ETag == etag {
		t.Fatal("etag did not change")
	}
	data, err := os.ReadFile(filepath.Join(dir, ".ssh", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# keep") || !strings.Contains(string(data), "UnknownThing value") || !strings.Contains(string(data), "new.example") {
		t.Fatalf("lossless update failed: %s", data)
	}
	if _, err := repo.Update(etag, nil, "prod", Edit{HostAlias: "prod", HostName: &value}); err != ErrPreconditionFailed {
		t.Fatalf("expected etag conflict, got %v", err)
	}
	_ = doc
}

func TestValidAliasRequiresLeadingAsciiLetter(t *testing.T) {
	valid := []string{"prod", "A1", "host.example", "host_name", "host-name", strings.Repeat("a", 255)}
	invalid := []string{"", "1host", "_host", ".host", "-host", strings.Repeat("a", 256), "host/one", "host name", "éhost"}
	for _, value := range valid {
		if !validAlias(value) {
			t.Errorf("validAlias(%q) = false", value)
		}
	}
	for _, value := range invalid {
		if validAlias(value) {
			t.Errorf("validAlias(%q) = true", value)
		}
	}
}

func TestCanonicalKeyUsesOpenSSHSpelling(t *testing.T) {
	want := map[string]string{
		"hostname":              "HostName",
		"user":                  "User",
		"port":                  "Port",
		"identityfile":          "IdentityFile",
		"identitiesonly":        "IdentitiesOnly",
		"stricthostkeychecking": "StrictHostKeyChecking",
		"userknownhostsfile":    "UserKnownHostsFile",
		"serveraliveinterval":   "ServerAliveInterval",
	}
	for key, expected := range want {
		if got := canonicalKey(key); got != expected {
			t.Errorf("canonicalKey(%q) = %q, want %q", key, got, expected)
		}
	}
}

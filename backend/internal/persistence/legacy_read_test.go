package persistence

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestLegacyDiscovery(t *testing.T) {
	state := t.TempDir()
	putLegacyTestFile(t, filepath.Join(state, "uploads", "upload-abandoned", "part"), []byte("stage only"))
	if loc, err := FindLegacyLocation(state, ""); err != nil || loc.Present {
		t.Fatalf("staging mistaken for records: %+v, %v", loc, err)
	}
	for _, rel := range []string{"agent-endpoints.json", "audit/connection-instances/draft/metadata.json", "uploads/upload-01.json"} {
		t.Run(rel, func(t *testing.T) {
			state := t.TempDir()
			inner := filepath.Join(state, "state")
			putLegacyTestFile(t, filepath.Join(inner, rel), []byte("{}"))
			loc, err := FindLegacyLocation(state, "")
			if err != nil || !loc.Present || loc.Root != inner {
				t.Fatalf("record-only source not found: %+v, %v", loc, err)
			}
			if _, err := FindLegacyLocation(state, state); err == nil {
				t.Fatal("explicit root omitted committed legacy data")
			}
			putLegacyTestFile(t, filepath.Join(state, "messages.json"), []byte("{}"))
			if _, err := FindLegacyLocation(state, inner); !errors.Is(err, ErrAmbiguousStateLayout) {
				t.Fatalf("accepted competing roots: %v", err)
			}
		})
	}
}

func TestLegacyRejectsPartialAndUnsafeSource(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"migrations/0.2-partial/in-progress", "unfinished legacy migration"},
		{"migrations/0.2-partial/auth-sessions.json", "missing a completion record"},
		{".connection-instances-migration-partial/metadata.json", "unfinished legacy connection migration"},
		{"sessions/00000000-0000-4000-8000-000000000001.snapshot", "orphan snapshot"},
		{"2fa-secret", "invalid legacy 2fa-secret"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			state := t.TempDir()
			putLegacyTestFile(t, filepath.Join(state, tc.file), []byte("{}"))
			loc, err := FindLegacyLocation(state, "")
			if err != nil || !loc.Present {
				t.Fatalf("partial legacy state hidden: %+v, %v", loc, err)
			}
			if _, err := ReadLegacy(loc, ""); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wrong error for partial source: %v", err)
			}
		})
	}
	t.Run("symlinked root", func(t *testing.T) {
		state := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(state, "state")); err != nil {
			t.Fatal(err)
		}
		if _, err := FindLegacyLocation(state, ""); err == nil {
			t.Fatal("an unsafe nested root was accepted")
		}
	})
}

func TestLegacyAuthV1KeepsLayoutWithoutRevivingUnboundCredentials(t *testing.T) {
	state := t.TempDir()
	now := time.Now().UTC()
	login := legacyAuthSession{ID: "00000000-0000-4000-8000-000000000001", PasswordFingerprint: strings.Repeat("a", 64),
		RefreshTokenHash: strings.Repeat("b", 64), CreatedAt: now, LastSeenAt: now, RotatedAt: now, RefreshExpiresAt: now.Add(time.Hour),
		ConnectionInstanceOrder: []string{"00000000-0000-4000-8000-000000000003", "00000000-0000-4000-8000-000000000002"}}
	path := filepath.Join(state, "auth-sessions.json")
	writeAuth := func() {
		raw, err := json.Marshal(legacyAuthFile{FormatVersion: 1, Sessions: []legacyAuthSession{login}})
		if err != nil {
			t.Fatal(err)
		}
		putLegacyTestFile(t, path, raw)
	}
	writeAuth()
	loc, err := FindLegacyLocation(state, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := ReadLegacy(loc, "")
	layout := data.Layouts[login.ID]
	if err != nil || len(data.Auth) != 0 || layout.Revision != 1 || len(layout.GroupOrder) != 1 || layout.GroupOrder[0] != domain.UngroupedConnectionInstanceGroupID ||
		strings.Join(layout.UngroupedConnectionInstanceIDs, ",") != strings.Join(login.ConnectionInstanceOrder, ",") {
		t.Fatalf("legacy workspace order or login revocation not honored: %v", err)
	}
	login.RefreshTokenHash = "malformed"
	writeAuth()
	if _, err := ReadLegacy(loc, ""); err == nil {
		t.Fatal("malformed file-era login was accepted")
	}
}

func TestLegacyPreparedDefinitionJournalRequiresOriginalBytes(t *testing.T) {
	state := t.TempDir()
	config := filepath.Join(t.TempDir(), "config")
	options := filepath.Join(state, "ssh-connection-options.yaml")
	native := []byte("Host test\n  HostName example.test\n")
	before := []byte("formatVersion: 2\nconnections:\n  retired-host:\n    tmux:\n      enabled: true\n      sessionName: old\n")
	after := bytes.ReplaceAll(before, []byte("old"), []byte("new"))
	snapshot := func(data []byte) map[string]any { return map[string]any{"exists": true, "data": data} }
	journal, err := json.Marshal(map[string]any{"version": 1, "id": "fixture", "phase": "prepared", "config": snapshot(native), "options": snapshot(before)})
	if err != nil {
		t.Fatal(err)
	}
	putLegacyTestFile(t, filepath.Join(state, "definition-mutation.json"), journal)
	putLegacyTestFile(t, options, after)
	putLegacyTestFile(t, config, append(bytes.Clone(native), '#'))
	loc, err := FindLegacyLocation(state, "")
	if err != nil || !loc.Present {
		t.Fatalf("pure definition state not detected: %v", err)
	}
	for _, path := range []string{"", config} {
		if _, err := ReadLegacy(loc, path); err == nil {
			t.Fatal("prepared journal accepted a missing or changed native config")
		}
	}
	putLegacyTestFile(t, config, native)
	data, err := ReadLegacy(loc, config)
	if err != nil || data.Options["retired-host"].SessionName != "old" {
		t.Fatalf("prepared journal did not use its original options snapshot: %v", err)
	}
	contents, err := os.ReadFile(options)
	if err != nil || !bytes.Equal(contents, after) {
		t.Fatalf("legacy journal recovery changed source files: %v", err)
	}
}

func TestLegacyReadInterruptedArchive(t *testing.T) {
	header := SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 100, ThroughSequence: "9007199254740993"}
	payload := []byte("retired connection\r\n")
	raw, err := EncodeSnapshot(header, payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		active, audit bool
	}{
		{"both snapshots", true, true},
		{"active snapshot only", true, false},
		{"audit snapshot only", false, true},
		{"metadata only", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, active, audit := writeLegacyArchivePair(t)
			if tc.active {
				putLegacyTestFile(t, filepath.Join(active, "terminal.snapshot"), raw)
			}
			if tc.audit {
				putLegacyTestFile(t, filepath.Join(audit, "terminal.snapshot"), raw)
			}
			data, err := ReadLegacy(loc, "")
			if err != nil || len(data.Instances) != 2 {
				t.Fatalf("expected both projections: %v", err)
			}
			for i, wantSnapshot := range []bool{tc.active, tc.audit} {
				instance := data.Instances[i]
				if instance.Archived != (i == 1) || instance.Metadata.ID != filepath.Base(active) || instance.Metadata.Lifecycle != "exited" {
					t.Fatal("archive overlap changed projection or metadata")
				}
				got, body, err := instance.Snapshot()
				if err != nil || (got != nil) != wantSnapshot || wantSnapshot && (got.ThroughSequence != header.ThroughSequence || !bytes.Equal(body, payload)) {
					t.Fatalf("snapshot not preserved for archived=%t: %v", instance.Archived, err)
				}
			}
		})
	}
}

func TestLegacyReadRejectsUnprovenArchiveOverlap(t *testing.T) {
	header := SnapshotHeader{Cols: 80, Rows: 24, ThroughSequence: "17"}
	for _, tc := range []struct{ name, want string }{
		{"different metadata", "conflicting legacy active and audit metadata"},
		{"missing metadata", "metadata is missing"},
		{"different snapshots", "conflicting legacy active and audit snapshots"},
		{"corrupt snapshot", "invalid legacy terminal snapshot"},
		{"flat overlap", "duplicate legacy flat connection instance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, active, audit := writeLegacyArchivePair(t)
			switch tc.name {
			case "different metadata":
				putLegacyTestFile(t, filepath.Join(audit, "metadata.json"), legacyPairMetadata("changed"))
			case "missing metadata":
				if err := os.Remove(filepath.Join(active, "metadata.json")); err != nil {
					t.Fatal(err)
				}
			case "different snapshots":
				for _, dir := range []string{active, audit} {
					raw, err := EncodeSnapshot(header, []byte(dir))
					if err != nil {
						t.Fatal(err)
					}
					putLegacyTestFile(t, filepath.Join(dir, "terminal.snapshot"), raw)
				}
			case "corrupt snapshot":
				putLegacyTestFile(t, filepath.Join(audit, "terminal.snapshot"), []byte("bad snapshot"))
			case "flat overlap":
				flat := bytes.Replace(legacyPairMetadata("original"), []byte(`"connectionInstanceId"`), []byte(`"id"`), 1)
				putLegacyTestFile(t, filepath.Join(loc.Root, "sessions", filepath.Base(active)+".json"), flat)
			}
			if _, err := ReadLegacy(loc, ""); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wrong outcome for unreconciled archive: %v", err)
			}
		})
	}
}

const legacyPairID = "00000000-0000-4000-8000-000000000001"

func legacyPairMetadata(title string) []byte {
	return []byte(`{"formatVersion":1,"connectionInstanceId":"` + legacyPairID + `","automaticTitle":"` + title + `","lifecycle":"exited","initialCwd":"/workspace","cwd":"/workspace","cols":80,"rows":24,"createdAt":"2024-04-01T12:00:00Z","updatedAt":"2024-04-01T12:00:01Z"}`)
}

func writeLegacyArchivePair(t *testing.T) (LegacyLocation, string, string) {
	t.Helper()
	root := t.TempDir()
	active := filepath.Join(root, "connection-instances", legacyPairID)
	audit := filepath.Join(root, "audit", "connection-instances", legacyPairID)
	for _, dir := range []string{active, audit} {
		putLegacyTestFile(t, filepath.Join(dir, "metadata.json"), legacyPairMetadata("original"))
	}
	loc, err := FindLegacyLocation(root, "")
	if err != nil || !loc.Present {
		t.Fatalf("cannot discover archive overlap: %v", err)
	}
	return loc, active, audit
}

func putLegacyTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

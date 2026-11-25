package db

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func TestSQLiteImportLegacyFiles(t *testing.T) {
	for _, layout := range []string{"direct", "state-child"} {
		t.Run(layout, func(t *testing.T) {
			s, _ := sqliteStore(t)
			testLegacyImport(t, s, layout == "state-child")
		})
	}
}

func TestPostgresImportLegacyFiles(t *testing.T) {
	s, _ := postgresStore(t)
	testLegacyImport(t, s, true)
}

func TestSQLiteImportLegacyInterruptedArchive(t *testing.T) {
	s, _ := sqliteStore(t)
	testLegacyImportInterruptedArchive(t, s)
}

func TestPostgresImportLegacyInterruptedArchive(t *testing.T) {
	s, _ := postgresStore(t)
	testLegacyImportInterruptedArchive(t, s)
}

func testLegacyImportInterruptedArchive(t *testing.T, s *Store) {
	t.Helper()
	ctx := t.Context()
	state := t.TempDir()
	source, err := persistence.New(state)
	requireTransferTestSuccess(t, err)
	const title = "Retired connection"
	payload := string(legacyCheckpoint)
	header := snapshotFixture()
	for _, id := range []domain.ConnectionInstanceID{firstInstance, secondInstance} {
		meta := instanceFixture(id)
		meta.AutomaticTitle, meta.Lifecycle = title, "exited"
		requireTransferTestSuccess(t, source.SaveConnectionInstance(meta))
		requireTransferTestSuccess(t, source.SaveSnapshot(id.String(), header, legacyCheckpoint))
		requireTransferTestSuccess(t, source.ArchiveConnectionInstance(id.String()))
	}
	// First: an audit checkpoint survived loss of the active checkpoint.
	// Second: copying stopped after audit metadata, before the checkpoint.
	path := source.AuditConnectionSnapshotPath(firstInstance.String())
	requireTransferTestSuccess(t, os.Remove(source.ConnectionSnapshotPath(firstInstance.String())))
	requireTransferTestSuccess(t, os.Remove(source.AuditConnectionSnapshotPath(secondInstance.String())))
	data := readLegacyFixture(t, state, state)
	if len(data.Instances) != 4 {
		t.Fatal("inspection lost an active or audit projection")
	}

	original, err := os.ReadFile(path)
	requireTransferTestSuccess(t, err)
	changed, err := persistence.EncodeSnapshot(header, []byte("changed after inspection"))
	requireTransferTestSuccess(t, err)
	writeLegacyBytes(t, path, changed)
	if err := s.ImportLegacy(ctx, data); err == nil || !strings.Contains(err.Error(), "snapshot changed") {
		t.Fatalf("import accepted an audit checkpoint changed after inspection: %v", err)
	}
	assertTransferTargetEmpty(t, s)
	writeLegacyBytes(t, path, original)

	requireTransferTestSuccess(t, s.ImportLegacy(ctx, data))
	requireTransferTestSuccess(t, s.VerifyLegacy(ctx, data))
	active, err := s.ListConnectionInstances(ctx)
	if err != nil || len(active) != 2 {
		t.Fatalf("import did not preserve both active rows: %v", err)
	}
	checkAudit(t, findAudit(t, s, firstInstance), title, &payload)
	checkAudit(t, findAudit(t, s, secondInstance), title, nil)
	if _, _, err := s.LoadSnapshot(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("import unexpectedly populated the absent active checkpoint: %v", err)
	}
	if h, body, err := s.LoadSnapshot(ctx, secondInstance); err != nil || h.ThroughSequence != header.ThroughSequence || !bytes.Equal(body, legacyCheckpoint) {
		t.Fatalf("active checkpoint changed: %v", err)
	}

	// Startup retirement must keep the audit-only checkpoint and move the
	// active-only checkpoint into audit, without leaving either active row.
	for _, id := range []domain.ConnectionInstanceID{firstInstance, secondInstance} {
		requireTransferTestSuccess(t, s.RetireConnectionInstance(ctx, id))
		if _, err := s.LoadConnectionInstance(ctx, id); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("connection instance still active after retirement: %v", err)
		}
		checkAudit(t, findAudit(t, s, id), title, &payload)
	}
	if current := readLegacyFixture(t, state, state); current.Fingerprint != data.Fingerprint {
		t.Fatal("inspection, import, or retirement modified the source copy")
	}
}

func testLegacyImport(t *testing.T, s *Store, nested bool) {
	t.Helper()
	ctx := t.Context()
	state := t.TempDir()
	root := state
	if nested {
		root = filepath.Join(state, "state")
	}
	writeLegacyFixture(t, state, root)
	data := readLegacyFixture(t, state, root)
	assertDecodedLegacyFixture(t, data)
	owner := strings.Repeat("10", 16)
	if err := s.PrepareServing(ctx, owner, true, data.Fingerprint); !errors.Is(err, ErrImportRequired) {
		t.Fatalf("first startup without import: %v", err)
	}
	if err := s.VerifyLegacy(ctx, data); !errors.Is(err, errLegacyTarget) {
		t.Fatalf("verification without import: %v", err)
	}

	// A changed snapshot must roll back everything written before it, including
	// auth, options, and notifications; the completion record must stay empty.
	path := filepath.Join(root, "sessions", flatV2ID+".snapshot")
	original, err := os.ReadFile(path)
	requireTransferTestSuccess(t, err)
	changed := bytes.Clone(original)
	changed[len(changed)-1] ^= 1
	writeLegacyBytes(t, path, changed)
	if err := s.ImportLegacy(ctx, data); err == nil || !strings.Contains(err.Error(), "snapshot changed") {
		t.Fatalf("import accepted a changed snapshot: %v", err)
	}
	assertTransferTargetEmpty(t, s)
	writeLegacyBytes(t, path, original)

	requireTransferTestSuccess(t, s.ImportLegacy(ctx, data))
	requireTransferTestSuccess(t, s.VerifyLegacy(ctx, data))
	before := exportTransferFixture(t, s)
	requireTransferTestSuccess(t, s.ImportLegacy(ctx, data))
	if !bytes.Equal(before, exportTransferFixture(t, s)) {
		t.Fatal("reimport rewrote database records")
	}
	if current := readLegacyFixture(t, state, root); current.Fingerprint != data.Fingerprint {
		t.Fatal("inspection or import modified the source")
	}
	assertImportedLegacyFixture(t, s)

	optionsPath := filepath.Join(state, "ssh-connection-options.yaml")
	options, err := os.ReadFile(optionsPath)
	requireTransferTestSuccess(t, err)
	writeLegacyBytes(t, optionsPath, append(options, []byte("\n# changed copy\n")...))
	mismatch := readLegacyFixture(t, state, root)
	for _, op := range []func() error{
		func() error { return s.ImportLegacy(ctx, mismatch) },
		func() error { return s.VerifyLegacy(ctx, mismatch) },
	} {
		if err := op(); !errors.Is(err, errLegacyTarget) {
			t.Fatalf("accepted mismatched source: %v", err)
		}
	}
	if err := s.PrepareServing(ctx, owner, true, mismatch.Fingerprint); !errors.Is(err, ErrImportRequired) {
		t.Fatalf("startup accepted mismatched source: %v", err)
	}
	requireTransferTestSuccess(t, s.PrepareServing(ctx, owner, true, data.Fingerprint))
	if err := s.ImportLegacy(ctx, data); !errors.Is(err, errLegacyTarget) {
		t.Fatalf("file import after serving: %v", err)
	}
}

func assertDecodedLegacyFixture(t *testing.T, data persistence.LegacyData) {
	t.Helper()
	if data.Enrollment == nil || data.Enrollment.LastAcceptedTimeStep != 81 || len(data.Auth) != 1 || data.Auth[0].ID != sampleAuth().ID ||
		len(data.Layouts) != 1 || len(data.Instances) != 4 || len(data.Endpoints) != 2 || len(data.Options) != 1 ||
		len(data.Uploads) != 1 || len(data.Messages) != 2 || len(data.Subscriptions) != 2 || len(data.Preferences) != 2 {
		t.Fatal("legacy inspection lost records or revived a revoked login session")
	}
	if data.Uploads[0].Status != "failed" || data.Uploads[0].StagingPath != "" || data.Uploads[0].Files[0].StagedPath != "" || len(data.Uploads[0].Failures) != 1 {
		t.Fatal("legacy import retained a staging path or a live upload")
	}
}

func assertImportedLegacyFixture(t *testing.T, s *Store) {
	t.Helper()
	ctx := t.Context()
	if err := s.SaveEnrollment(ctx, sampleEnrollment(81)); !errors.Is(err, ErrStaleEnrollment) {
		t.Fatalf("imported enrollment accepted a replay: %v", err)
	}
	layout, found, err := s.LoadWorkspaceLayout(ctx, domain.AuthenticationSessionID(sampleAuth().ID))
	if err != nil || !found || !sameJSON(layout, workspaceTestLayout(1)) {
		t.Fatal("workspace order or revision was not restored")
	}
	if got, err := s.MessageState(); err != nil || got != (domain.MessageState{LatestSequence: 3, Revision: 5, ReadThroughSequence: 1, UnreadCount: 1}) {
		t.Fatalf("message counters changed: %+v, %v", got, err)
	}
	meta, err := s.LoadConnectionInstance(ctx, flatV2ID)
	if err != nil || meta.Title != "Version two" || meta.AutomaticTitle != "Automatic" || meta.FormatVersion != persistence.ConnectionFormatVersion {
		t.Fatalf("flat version-2 metadata not converted: %v", err)
	}
	meta, err = s.LoadConnectionInstance(ctx, thirdInstance)
	if err != nil || meta.Title != "Version one" {
		t.Fatalf("flat version-1 metadata not converted: %v", err)
	}
	header, payload, err := s.LoadSnapshot(ctx, flatV2ID)
	if err != nil || header.ThroughSequence != "9007199254740993" || !bytes.Equal(payload, legacyCheckpoint) {
		t.Fatalf("flat version-2 checkpoint changed: %v", err)
	}
	if _, err := s.LoadConnectionInstance(ctx, secondInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("audit-only instance appeared as active: %v", err)
	}
	var archive instanceAuditRow
	requireTransferTestSuccess(t, s.DB.Take(&archive, &instanceAuditRow{ID: secondInstance.String()}).Error)
	if _, payload, err := persistence.DecodeSnapshot(archive.Snapshot); err != nil || !bytes.Equal(payload, legacyCheckpoint) {
		t.Fatalf("archived checkpoint changed: %v", err)
	}
}

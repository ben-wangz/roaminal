package db

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
)

func snapshotFixture() domain.SnapshotHeader {
	return domain.SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 24, ThroughSequence: "18446744073709551615"}
}

func TestSQLiteInstanceSnapshotLegacyEnvelope(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	header := snapshotFixture()
	header.ByteLength, header.SHA256 = 197, "not the digest"
	payload := []byte("\x1b[2Jhéllo\r\n\x00")
	want, err := persistence.EncodeSnapshot(header, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.LoadSnapshot(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) || s.PersistenceDegraded() {
		t.Fatalf("missing snapshot: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	// Saving a snapshot does not require metadata or create a listed instance.
	if err := s.SaveSnapshot(ctx, firstInstance, header, payload); err != nil {
		t.Fatal(err)
	}
	var row instanceSnapshotRow
	if err := s.DB.Where("id = ?", firstInstance.String()).Take(&row).Error; err != nil || !bytes.Equal(row.Data, want) {
		t.Fatalf("snapshot differs from the legacy envelope: %v", err)
	}
	wantHeader, _, err := persistence.DecodeSnapshot(want)
	if err != nil {
		t.Fatal(err)
	}
	gotHeader, gotPayload, err := s.LoadSnapshot(ctx, firstInstance)
	if err != nil || gotHeader != wantHeader || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("snapshot round trip: header=%+v, payload=%q, err=%v", gotHeader, gotPayload, err)
	}
	if list, err := s.ListConnectionInstances(ctx); err != nil || len(list) != 0 {
		t.Fatalf("snapshot-only instance was listed: %+v, %v", list, err)
	}
	if err := s.DeleteConnectionInstance(ctx, firstInstance); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.LoadSnapshot(ctx, firstInstance); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deletion did not remove snapshot-only data: %v", err)
	}
}

func TestSQLiteInstanceSnapshotRejectsInvalidAndCorruptData(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	header := snapshotFixture()
	if err := s.SaveSnapshot(ctx, firstInstance, header, []byte{0xff}); err == nil || !s.PersistenceDegraded() {
		t.Fatalf("invalid UTF-8: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	bad := header
	bad.ThroughSequence = "01"
	if err := s.SaveSnapshot(ctx, firstInstance, bad, nil); err == nil {
		t.Fatal("accepted invalid sequence")
	}
	payload := []byte("output")
	if err := s.SaveSnapshot(ctx, firstInstance, header, payload); err != nil || s.PersistenceDegraded() {
		t.Fatalf("replacement snapshot: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	encoded, err := persistence.EncodeSnapshot(header, payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-1] = 'X'
	if err := s.DB.Model(&instanceSnapshotRow{}).Where("id = ?", firstInstance.String()).Update("data", encoded).Error; err != nil {
		t.Fatal(err)
	}
	if _, got, err := s.LoadSnapshot(ctx, firstInstance); err == nil || !strings.Contains(err.Error(), "checksum") || got != nil || !s.PersistenceDegraded() {
		t.Fatalf("corrupt checksum: payload=%q, err=%v, degraded=%v", got, err, s.PersistenceDegraded())
	}
	if err := s.SaveSnapshot(ctx, firstInstance, header, nil); err != nil || s.PersistenceDegraded() {
		t.Fatalf("valid empty snapshot: err=%v, degraded=%v", err, s.PersistenceDegraded())
	}
	if gotHeader, got, err := s.LoadSnapshot(ctx, firstInstance); err != nil || gotHeader.ByteLength != 0 || len(got) != 0 {
		t.Fatalf("empty snapshot: header=%+v, payload=%q, err=%v", gotHeader, got, err)
	}
}

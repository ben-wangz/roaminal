package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"gorm.io/gorm"
)

func TestDatabaseTransferModelCoverage(t *testing.T) {
	// New application data must not silently disappear from a backup.
	want := map[string]bool{(authRow{}).TableName(): true, (enrollmentRow{}).TableName(): true}
	for _, model := range applicationModels() {
		switch model.(type) {
		case *installationRow, *repositoryLock, *definitionMutationRow:
			continue
		}
		want[model.(transferModel).TableName()] = true
	}
	for _, section := range transferTables {
		name := section.name()
		if !want[name] {
			t.Errorf("unexpected or repeated transfer table %s", name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("application table %s not in the transfer format", name)
	}
}

func TestSQLiteDatabaseTransfer(t *testing.T) {
	source, _ := sqliteStore(t)
	target, _ := sqliteStore(t)
	testDatabaseTransfer(t, source, target)
}

func testDatabaseTransfer(t *testing.T, source, target *Store) {
	t.Helper()
	ctx := context.Background()
	checkpoint := bytes.Repeat([]byte("line ☃\n"), 1<<17)
	legacy := strings.Repeat("a3", sha256.Size)
	if err := source.DB.Model(&installationRow{Key: primaryKey}).Update("legacy_fingerprint", legacy).Error; err != nil {
		t.Fatal("seed legacy fingerprint")
	}
	draft := seedDatabaseTransfer(t, source, checkpoint)
	requireTransferTestSuccess(t, source.PrepareServing(ctx, strings.Repeat("1a", 16), true, legacy))
	before, err := target.Installation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := source.Installation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backup := exportTransferFixture(t, source)
	if !bytes.Contains(backup, checkpoint) {
		t.Fatal("snapshot was not transferred as raw bytes")
	}
	corrupt := bytes.Clone(backup)
	corrupt[len(corrupt)-1] ^= 1
	if err := target.Import(ctx, bytes.NewReader(corrupt)); !errors.Is(err, errTransferChecksum) {
		t.Fatalf("import with corrupt checksum: %v", err)
	}
	assertTransferTargetEmpty(t, target)
	if err := target.Import(ctx, bytes.NewReader(backup)); err != nil {
		t.Fatal(err)
	}
	restored := exportTransferFixture(t, target)
	if !bytes.Equal(transferApplicationBytes(backup), transferApplicationBytes(restored)) {
		t.Fatal("restored application rows differ")
	}
	var installed installationRow
	if err := target.DB.Take(&installed, &installationRow{Key: primaryKey}).Error; err != nil || installed.ID != before.ID ||
		installed.TransferSourceID != origin.ID || installed.Served || installed.ServingOwner != "" || installed.LegacyFingerprint != legacy ||
		installed.TransferFingerprint != hex.EncodeToString(backup[len(backup)-sha256.Size:]) {
		t.Fatal("transfer completion or installation identity mismatch")
	}

	requireTransferTestSuccess(t, target.Import(ctx, bytes.NewReader(backup)))
	requireTransferTestSuccess(t, target.VerifyTransfer(ctx))
	changed := uploadTestRecord()
	changed.Status = "cancelled"
	requireTransferTestSuccess(t, target.SaveUpload(ctx, changed))
	if err := target.Import(ctx, bytes.NewReader(backup)); !errors.Is(err, ErrTransferVerification) {
		t.Fatalf("same backup accepted a changed destination: %v", err)
	}
	if err := target.VerifyTransfer(ctx); !errors.Is(err, ErrTransferVerification) {
		t.Fatalf("verified a changed destination: %v", err)
	}
	if err := target.PrepareServing(ctx, strings.Repeat("6b", 16), true, legacy); !errors.Is(err, ErrTransferVerification) {
		t.Fatalf("claimed a changed destination: %v", err)
	}
	assertUploadTestRecord(t, target, changed)
	requireTransferTestSuccess(t, target.SaveUpload(ctx, uploadTestRecord()))
	if err := target.Import(ctx, bytes.NewReader(backup[:len(backup)-1])); !errors.Is(err, errTransferFormat) {
		t.Fatalf("truncated retry: %v", err)
	}
	if err := source.SaveUpload(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if err := target.Import(ctx, bytes.NewReader(exportTransferFixture(t, source))); !errors.Is(err, errTransferTarget) {
		t.Fatalf("different backup from the same source: %v", err)
	}
	requireTransferTestSuccess(t, target.PrepareServing(ctx, strings.Repeat("6b", 16), true, legacy))
	assertTransferRuntimeState(t, target, draft)
	if err := target.Import(ctx, bytes.NewReader(backup)); !errors.Is(err, errTransferTarget) {
		t.Fatalf("restore after serving: %v", err)
	}
}

func seedDatabaseTransfer(t *testing.T, s *Store, checkpoint []byte) domain.MessageDraft {
	t.Helper()
	ctx := context.Background()
	auth := sampleAuth()
	auth.CreatedAt = auth.CreatedAt.Add(123)
	auth.LastSeenAt = auth.LastSeenAt.Add(456)
	auth.RefreshExpiresAt = auth.RefreshExpiresAt.Add(789)
	auth.RotatedAt = auth.RotatedAt.Add(987)
	requireTransferTestSuccess(t, s.SaveEnrollment(ctx, sampleEnrollment(81)))
	requireTransferTestSuccess(t, s.SaveAuth(ctx, []domain.AuthSessionRecord{auth}))
	requireTransferTestSuccess(t, s.ImportWorkspaceLayout(ctx, domain.AuthenticationSessionID(auth.ID), workspaceTestLayout(1<<63)))
	requireTransferTestSuccess(t, s.SaveUpload(ctx, uploadTestRecord()))

	header := domain.SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 1000, ThroughSequence: "9007199254740993"}
	saveInstanceFixture(t, s, instanceFixture(firstInstance))
	requireTransferTestSuccess(t, s.SaveSnapshot(ctx, firstInstance, header, checkpoint))
	requireTransferTestSuccess(t, s.ImportArchivedConnectionInstance(ctx, instanceFixture(secondInstance), &header, []byte("retired scrollback")))
	requireTransferTestSuccess(t, s.ImportArchivedConnectionInstance(ctx, instanceFixture(thirdInstance), nil, nil))

	// Start above the signed integer limit and leave a gap in retained rows.
	requireTransferTestSuccess(t, saveMessageState(s.DB, messageStateRow{ID: messageStateID, LatestSequence: 1 << 63, Revision: (1 << 63) + 3}))
	draft := messageDraft(71, time.Now().UTC())
	stored, _, err := s.AppendMessage(draft)
	requireTransferTestSuccess(t, err)
	_, _, err = s.AppendMessage(messageDraft(72, time.Now().UTC()))
	requireTransferTestSuccess(t, err)
	_, _, err = s.DeleteMessage(messageDraft(72, time.Now().UTC()).MessageID)
	requireTransferTestSuccess(t, err)
	_, err = s.MarkMessagesReadThrough(stored.Sequence)
	requireTransferTestSuccess(t, err)

	for _, n := range []int{8, 1, 6} {
		_, err = s.UpsertPushSubscription(ctx, pushFixture(t, n))
		requireTransferTestSuccess(t, err)
		_, err = s.UpsertNotificationPreference(ctx, preferenceFixture(n))
		requireTransferTestSuccess(t, err)
	}
	_, err = s.DeletePushSubscriptionByID(ctx, pushFixture(t, 1).ID)
	requireTransferTestSuccess(t, err)
	_, endpoint := agentEndpointFixture(t)
	endpoint.Targets["work"] = domain.AgentTargetState{SessionName: "work", LastSequence: 7, LastTransitionKey: "runtime\x002"}
	requireTransferTestSuccess(t, s.ImportAgentEndpoint(ctx, endpoint))
	requireTransferTestSuccess(t, s.ConnectionOptions().UpdateAlias("alpha", connectionoptions.Tmux{Enabled: true, SessionName: "work"}, true))
	requireTransferTestSuccess(t, s.ConnectionOptions().UpdateAlias("beta", connectionoptions.Tmux{}, true))
	return draft
}

func assertTransferRuntimeState(t *testing.T, s *Store, draft domain.MessageDraft) {
	t.Helper()
	ctx := context.Background()
	if err := s.SaveEnrollment(ctx, sampleEnrollment(81)); !errors.Is(err, ErrStaleEnrollment) {
		t.Fatalf("replayed TOTP step was not rejected: %v", err)
	}
	state, err := s.MessageState()
	if err != nil || state.LatestSequence != 1<<63+2 || state.Revision != 1<<63+7 || state.ReadThroughSequence != 1<<63+1 || state.UnreadCount != 0 {
		t.Fatalf("message counters: %+v, %v", state, err)
	}
	message, duplicate, err := s.AppendMessage(draft)
	if err != nil || !duplicate || message.Sequence != 1<<63+1 {
		t.Fatalf("message deduplication: duplicate=%v, sequence=%d, err=%v", duplicate, message.Sequence, err)
	}
	message, duplicate, err = s.AppendMessage(messageDraft(73, time.Now().UTC()))
	if err != nil || duplicate || message.Sequence != 1<<63+3 {
		t.Fatalf("message advance: duplicate=%v, sequence=%d, err=%v", duplicate, message.Sequence, err)
	}
	_, err = s.UpsertPushSubscription(ctx, pushFixture(t, 9))
	requireTransferTestSuccess(t, err)
	_, err = s.UpsertNotificationPreference(ctx, preferenceFixture(9))
	requireTransferTestSuccess(t, err)
	push, err := s.ListPushSubscriptions(ctx)
	if err != nil || len(push) != 3 || push[0].ID != pushFixture(t, 8).ID || push[1].ID != pushFixture(t, 6).ID || push[2].ID != pushFixture(t, 9).ID {
		t.Fatal("push ordering or new position was not retained")
	}
}

func exportTransferFixture(t *testing.T, s *Store) []byte {
	t.Helper()
	var buffer transferChunkWriter
	if err := s.Export(context.Background(), &buffer); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type transferChunkWriter struct{ bytes.Buffer }

func (w *transferChunkWriter) Write(p []byte) (int, error) {
	if len(p) > transferChunk {
		return 0, errors.New("write exceeds chunk size")
	}
	return w.Buffer.Write(p)
}

func transferApplicationBytes(data []byte) []byte {
	manifestSize := int(binary.BigEndian.Uint32(data[len(transferMagic):]))
	return data[len(transferMagic)+4+manifestSize : len(data)-sha256.Size]
}

func requireTransferTestSuccess(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertTransferTargetEmpty(t *testing.T, s *Store) {
	t.Helper()
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		target, err := lockInstallation(tx)
		if err != nil || target.Served || target.LegacyFingerprint != "" || target.TransferFingerprint != "" || target.TransferSourceID != "" {
			return errors.New("destination installation record changed")
		}
		return requireEmptyApplication(tx)
	}); err != nil {
		t.Fatal(err)
	}
}

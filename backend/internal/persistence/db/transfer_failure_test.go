package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSQLiteDatabaseTransferRollback(t *testing.T) {
	ctx := context.Background()
	source, _ := sqliteStore(t)
	target, _ := sqliteStore(t)
	seedDatabaseTransfer(t, source, []byte("scrollback\n"))
	backup := exportTransferFixture(t, source)
	badChecksum := bytes.Clone(backup)
	badChecksum[len(badChecksum)-1] ^= 1
	oversize := bytes.Clone(backup)
	binary.BigEndian.PutUint32(oversize[len(transferMagic):], transferManifestLimit+1)
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"wrong version", append([]byte("ROAMINAL-DB-TRANSFER/2\n"), backup[len(transferMagic):]...), errTransferFormat},
		{"oversize manifest", oversize, errTransferFormat},
		{"truncation", backup[:len(backup)-1], errTransferFormat},
		{"checksum", badChecksum, errTransferChecksum},
		{"trailing data", append(bytes.Clone(backup), 1), errTransferFormat},
		{"verified invalid row", malformedMessageTransfer(t, backup), errTransferFormat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := target.Import(ctx, bytes.NewReader(tc.data)); !errors.Is(err, tc.want) {
				t.Fatalf("import: got %v, want %v", err, tc.want)
			}
			assertTransferTargetEmpty(t, target)
		})
	}
	if err := target.Import(ctx, bytes.NewReader(backup)); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

func malformedMessageTransfer(t *testing.T, backup []byte) []byte {
	t.Helper()
	bad := bytes.Clone(backup)
	needle := []byte(`"agentType":"codex"`)
	offset := bytes.Index(bad, needle)
	if offset < 0 {
		t.Fatal("missing fixture message")
	}
	bad[offset+len(needle)-2] = 'y'
	body := bad[:len(bad)-sha256.Size]
	digest := sha256.Sum256(body)
	copy(bad[len(body):], digest[:])
	return bad
}

func TestSQLiteDatabaseTransferRejectsNonemptyDestination(t *testing.T) {
	ctx := context.Background()
	source, _ := sqliteStore(t)
	target, _ := sqliteStore(t)
	backup := exportTransferFixture(t, source)
	if err := target.SaveUpload(ctx, uploadTestRecord()); err != nil {
		t.Fatal(err)
	}
	if err := target.Import(ctx, bytes.NewReader(backup)); !errors.Is(err, errTransferTarget) {
		t.Fatalf("nonempty restore target: %v", err)
	}
	assertUploadTestRecord(t, target, uploadTestRecord())
	if err := source.Import(ctx, bytes.NewReader(backup)); !errors.Is(err, errTransferTarget) {
		t.Fatalf("restore to source installation: %v", err)
	}
}

func TestSQLiteDatabaseTransferRejectsPendingDefinitionMutation(t *testing.T) {
	s, _ := sqliteStore(t)
	// Export refuses even malformed intents rather than quietly dropping them.
	row := definitionMutationRow{Key: mutationKey, Updates: []byte{},
		Before: mutationRevisionRow{Digest: []byte{}}, After: mutationRevisionRow{Digest: []byte{}}}
	if err := s.DB.Create(&row).Error; err != nil {
		t.Fatal("seed pending intent")
	}
	var out bytes.Buffer
	if err := s.Export(context.Background(), &out); !errors.Is(err, errTransferPending) || out.Len() != 0 {
		t.Fatalf("export with pending mutation: wrote %d bytes, err=%v", out.Len(), err)
	}
}

func TestSQLiteDatabaseTransferRedactsIOErrors(t *testing.T) {
	s, _ := sqliteStore(t)
	ctx := context.Background()
	secret := errors.New("credential material from stream")
	for name, err := range map[string]error{
		"read": s.Import(ctx, transferErrorIO{err: secret}), "write": s.Export(ctx, transferErrorIO{err: secret}),
		"short write": s.Export(ctx, transferErrorIO{}),
	} {
		if err == nil || errors.Is(err, secret) || strings.Contains(err.Error(), secret.Error()) {
			t.Errorf("%s did not return a redacted error", name)
		}
	}
	assertTransferTargetEmpty(t, s)
}

type transferErrorIO struct{ err error }

func (s transferErrorIO) Read([]byte) (int, error) {
	if s.err == nil {
		return 0, io.EOF
	}
	return 0, s.err
}

func (s transferErrorIO) Write([]byte) (int, error) { return 0, s.err }

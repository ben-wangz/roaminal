package db

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"unicode/utf8"
)

// V1: magic, length-prefixed JSON manifest, ordered table sections, end marker,
// SHA-256 (32 raw bytes), EOF. Sections contain length-prefixed JSON rows and
// end with an empty frame. Snapshot/upload rows also carry a raw bytes frame.
// Every length is uint32 big endian. The digest covers all preceding bytes.
const (
	transferMagic         = "ROAMINAL-DB-TRANSFER/1\n"
	transferEnd           = "ROAMINAL-DB-TRANSFER-END\n"
	transferManifestLimit = 1024
	transferRecordLimit   = 16 << 20
	transferUploadLimit   = 128 << 20
	transferChunk         = 32 << 10
)

var (
	errTransferRead     = errors.New("read database transfer failed")
	errTransferWrite    = errors.New("write database transfer failed")
	errTransferFormat   = errors.New("invalid, unsupported, or incomplete database transfer")
	errTransferChecksum = errors.New("database transfer checksum mismatch")
)

type transferOutput struct {
	ctx    context.Context
	stream io.Writer
	digest hash.Hash
}

func newTransferOutput(ctx context.Context, w io.Writer) *transferOutput {
	return &transferOutput{ctx: ctx, stream: w, digest: sha256.New()}
}

func (out *transferOutput) write(p []byte, hashed bool) error {
	for len(p) != 0 {
		if err := out.ctx.Err(); err != nil {
			return err
		}
		chunk := p[:min(len(p), transferChunk)]
		if n, err := out.stream.Write(chunk); err != nil || n != len(chunk) {
			return errTransferWrite
		}
		if hashed {
			_, _ = out.digest.Write(chunk)
		}
		p = p[len(chunk):]
	}
	return nil
}

func (out *transferOutput) frame(data []byte, limit int) error {
	if len(data) > limit {
		return errTransferFormat
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	if err := out.write(length[:], true); err != nil {
		return err
	}
	return out.write(data, true)
}

func (out *transferOutput) json(value any, limit int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errTransferFormat
	}
	return out.frame(data, limit)
}

func (out *transferOutput) writeManifest(manifest transferManifest) error {
	if err := out.write([]byte(transferMagic), true); err != nil {
		return err
	}
	return out.json(manifest, transferManifestLimit)
}

func (out *transferOutput) finish() error {
	if err := out.write([]byte(transferEnd), true); err != nil {
		return err
	}
	return out.write(out.digest.Sum(nil), false)
}

type transferInput struct {
	ctx    context.Context
	stream *bufio.Reader
	digest hash.Hash
}

func newTransferInput(ctx context.Context, r io.Reader) *transferInput {
	return &transferInput{ctx: ctx, stream: bufio.NewReaderSize(r, 2*transferChunk), digest: sha256.New()}
}

func (in *transferInput) read(p []byte, hashed bool) error {
	for len(p) != 0 {
		if err := in.ctx.Err(); err != nil {
			return err
		}
		chunk := p[:min(len(p), transferChunk)]
		n, err := in.stream.Read(chunk)
		if errors.Is(err, io.EOF) {
			return errTransferFormat
		}
		if err != nil || n == 0 {
			return errTransferRead
		}
		if hashed {
			_, _ = in.digest.Write(chunk[:n])
		}
		p = p[n:]
	}
	return nil
}

func (in *transferInput) marker(want string) error {
	data := make([]byte, len(want))
	if err := in.read(data, true); err != nil {
		return err
	}
	if string(data) != want {
		return errTransferFormat
	}
	return nil
}

func (in *transferInput) frame(limit int) ([]byte, error) {
	var length [4]byte
	if err := in.read(length[:], true); err != nil {
		return nil, err
	}
	count := binary.BigEndian.Uint32(length[:])
	if count > uint32(limit) {
		return nil, errTransferFormat
	}
	if count == 0 {
		return nil, nil
	}
	if count > transferChunk {
		// Don't allocate a large record based on a length prefix alone.
		if _, err := in.stream.Peek(transferChunk); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errTransferFormat
			}
			return nil, errTransferRead
		}
	}
	data := make([]byte, int(count))
	if err := in.read(data, true); err != nil {
		return nil, err
	}
	return data, nil
}

func (in *transferInput) readManifest() (transferManifest, error) {
	if err := in.marker(transferMagic); err != nil {
		return transferManifest{}, err
	}
	data, err := in.frame(transferManifestLimit)
	if err != nil {
		return transferManifest{}, err
	}
	var manifest transferManifest
	if !decodeTransferJSON(data, &manifest) || manifest.SchemaVersion != schemaVersion || !validHex(manifest.SourceID, 16) ||
		(manifest.LegacyFingerprint != "" && !validHex(manifest.LegacyFingerprint, sha256.Size)) {
		return transferManifest{}, errTransferFormat
	}
	return manifest, nil
}

func (in *transferInput) finish() (string, error) {
	if err := in.marker(transferEnd); err != nil {
		return "", err
	}
	digest := in.digest.Sum(nil)
	var received [sha256.Size]byte
	if err := in.read(received[:], false); err != nil {
		return "", err
	}
	if !bytes.Equal(digest, received[:]) {
		return "", errTransferChecksum
	}
	if _, err := in.stream.ReadByte(); !errors.Is(err, io.EOF) {
		if err != nil {
			return "", errTransferRead
		}
		return "", errTransferFormat
	}
	return hex.EncodeToString(digest), nil
}

func decodeTransferJSON(data []byte, target any) bool {
	data = bytes.TrimSpace(data)
	return len(data) != 0 && data[0] == '{' && utf8.Valid(data) && decodeWorkspaceUploadPayload(data, target) == nil
}

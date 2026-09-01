package imagepreview

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

func (s *Service) ETag(request Request) (string, error) {
	request = request.normalized()
	if err := validateRequest(request); err != nil {
		return "", err
	}
	if !eligibleMIME(request.MIMEType) {
		return "", ErrUnsupported
	}
	digest := identityDigest(request)
	return `"` + hex.EncodeToString(digest[:]) + `"`, nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.ConnectionInstanceID) == "" || strings.TrimSpace(request.RootAbsolutePath) == "" || strings.TrimSpace(request.RootRevision) == "" || strings.TrimSpace(request.SourceToken) == "" || request.SourceSize < 0 || request.Open == nil {
		return fmt.Errorf("%w: source descriptor is incomplete", ErrUnavailable)
	}
	clean := path.Clean(request.RelativePath)
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: relative path is invalid", ErrUnavailable)
	}
	return nil
}

func validateCachedReader(reader io.ReadSeekCloser, expectedSize int64) error {
	if reader == nil || expectedSize < 20 {
		return errors.New("cache reader is invalid")
	}
	originalPosition, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	defer func() { _, _ = reader.Seek(originalPosition, io.SeekStart) }()
	actualSize, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if actualSize != expectedSize {
		return errors.New("cache entry size changed")
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var header [12]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return err
	}
	if !bytes.Equal(header[:4], []byte("RIFF")) || !bytes.Equal(header[8:], []byte("WEBP")) {
		return errors.New("cache entry is not WebP")
	}
	if uint64(binary.LittleEndian.Uint32(header[4:8]))+8 != uint64(actualSize) {
		return errors.New("cache entry RIFF size is invalid")
	}

	var chunkHeader [8]byte
	position := int64(12)
	hasImageChunk := false
	for position < actualSize {
		if actualSize-position < int64(len(chunkHeader)) {
			return errors.New("cache entry has a truncated chunk header")
		}
		if _, err := io.ReadFull(reader, chunkHeader[:]); err != nil {
			return err
		}
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		payloadEnd := position + int64(len(chunkHeader)) + chunkSize
		if payloadEnd < position || payloadEnd > actualSize {
			return errors.New("cache entry chunk exceeds RIFF bounds")
		}
		paddedEnd := payloadEnd
		if chunkSize%2 != 0 {
			paddedEnd++
		}
		if paddedEnd > actualSize {
			return errors.New("cache entry chunk padding exceeds RIFF bounds")
		}
		if bytes.Equal(chunkHeader[:4], []byte("VP8 ")) || bytes.Equal(chunkHeader[:4], []byte("VP8L")) || bytes.Equal(chunkHeader[:4], []byte("VP8X")) {
			hasImageChunk = true
		}
		padding := paddedEnd - payloadEnd
		if _, err := io.CopyN(io.Discard, reader, chunkSize+padding); err != nil {
			return err
		}
		position = paddedEnd
	}
	if position != actualSize || !hasImageChunk {
		return errors.New("cache entry has no WebP image chunk")
	}
	return nil
}

func identityDigest(request Request) [sha256.Size]byte {
	fields := []string{
		request.ConnectionInstanceID,
		request.RootAbsolutePath,
		request.RootRevision,
		path.Clean(request.RelativePath),
		request.SourceToken,
		outputFormat,
		fmt.Sprintf("%d", request.SourceSize),
		fmt.Sprintf("%d", outputQuality),
		fmt.Sprintf("%d", outputEffort),
		PipelineVersion,
	}
	var input strings.Builder
	for _, field := range fields {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		input.Write(length[:])
		input.WriteString(field)
	}
	return sha256.Sum256([]byte(input.String()))
}

func cacheKeyPrefix(digest [sha256.Size]byte) string { return hex.EncodeToString(digest[:6]) }

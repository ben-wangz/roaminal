package persistence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"
)

func EncodeSnapshot(header SnapshotHeader, payload []byte) ([]byte, error) {
	if err := validateSnapshotHeader(header); err != nil {
		return nil, err
	}
	if len(payload) > SnapshotMaxSize {
		return nil, errors.New("snapshot exceeds 256 MiB")
	}
	if !utf8.Valid(payload) {
		return nil, errors.New("snapshot is not UTF-8")
	}
	header.ByteLength = len(payload)
	digest := sha256.Sum256(payload)
	header.SHA256 = hex.EncodeToString(digest[:])
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	return bytes.Join([][]byte{[]byte(SnapshotMagic), {'\n'}, headerBytes, {'\n'}, payload}, nil), nil
}

func DecodeSnapshot(data []byte) (SnapshotHeader, []byte, error) {
	if len(data) > SnapshotMaxSize+4096 {
		return SnapshotHeader{}, nil, errors.New("snapshot exceeds 256 MiB")
	}
	parts := bytes.SplitN(data, []byte{'\n'}, 3)
	if len(parts) != 3 || string(parts[0]) != SnapshotMagic {
		return SnapshotHeader{}, nil, errors.New("invalid snapshot magic")
	}
	var header SnapshotHeader
	if err := decodeStrict(parts[1], &header); err != nil {
		return SnapshotHeader{}, nil, fmt.Errorf("invalid snapshot header: %w", err)
	}
	if header.ByteLength != len(parts[2]) || len(parts[2]) > SnapshotMaxSize || !utf8.Valid(parts[2]) {
		return SnapshotHeader{}, nil, errors.New("invalid snapshot payload")
	}
	digest := sha256.Sum256(parts[2])
	if header.SHA256 != hex.EncodeToString(digest[:]) {
		return SnapshotHeader{}, nil, errors.New("snapshot checksum mismatch")
	}
	if err := validateSnapshotHeader(header); err != nil || !hex64Pattern.MatchString(header.SHA256) {
		return SnapshotHeader{}, nil, errors.New("invalid snapshot dimensions")
	}
	return header, parts[2], nil
}

func (s *Store) SaveSnapshot(id string, header SnapshotHeader, payload []byte) error {
	if !uuidPattern.MatchString(id) {
		return s.markSessionError(id, errors.New("invalid session id"))
	}
	data, err := EncodeSnapshot(header, payload)
	if err != nil {
		return s.markSessionError(id, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.SnapshotPath(id)), 0o700); err != nil {
		return s.markSessionError(id, err)
	}
	if err := s.atomicWrite(s.SnapshotPath(id), data); err != nil {
		return s.markSessionError(id, err)
	}
	s.clearSessionError(id)
	return nil
}

func (s *Store) LoadSnapshot(id string) (SnapshotHeader, []byte, error) {
	if !uuidPattern.MatchString(id) {
		return SnapshotHeader{}, nil, errors.New("invalid session id")
	}
	data, err := os.ReadFile(s.SnapshotPath(id))
	if err != nil {
		return SnapshotHeader{}, nil, err
	}
	header, payload, err := DecodeSnapshot(data)
	if err != nil {
		_ = s.quarantine(s.SnapshotPath(id), "corrupt")
		return SnapshotHeader{}, nil, err
	}
	return header, payload, nil
}

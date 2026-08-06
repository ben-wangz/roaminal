package persistence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	FormatVersion   = 1
	SnapshotMagic   = "ROAMINAL-SNAPSHOT/1"
	SnapshotMaxSize = 256 * 1024 * 1024
)

var ErrNotFound = os.ErrNotExist

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type ExecutionRecord struct {
	Command     string    `json:"command"`
	ExitCode    *int      `json:"exitCode"`
	Input       string    `json:"input"`
	Output      string    `json:"output"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DurationMs  int64     `json:"durationMs"`
	Truncated   bool      `json:"truncated"`
}

type SessionMeta struct {
	FormatVersion int               `json:"formatVersion"`
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	InitialCwd    string            `json:"initialCwd"`
	Cwd           string            `json:"cwd"`
	Cols          int               `json:"cols"`
	Rows          int               `json:"rows"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	Executions    []ExecutionRecord `json:"executions"`
}

type AuthSession struct {
	ID                  string    `json:"id"`
	PasswordFingerprint string    `json:"passwordFingerprint"`
	RefreshTokenHash    string    `json:"refreshTokenHash"`
	CreatedAt           time.Time `json:"createdAt"`
	LastSeenAt          time.Time `json:"lastSeenAt"`
	RefreshExpiresAt    time.Time `json:"refreshExpiresAt"`
	RotatedAt           time.Time `json:"rotatedAt"`
	UserAgent           string    `json:"userAgent"`
}

type AuthFile struct {
	FormatVersion int           `json:"formatVersion"`
	Sessions      []AuthSession `json:"sessions"`
}

type SnapshotHeader struct {
	Cols            int    `json:"cols"`
	Rows            int    `json:"rows"`
	ScrollbackLines int    `json:"scrollbackLines"`
	ThroughSequence string `json:"throughSequence"`
	ByteLength      int    `json:"byteLength"`
	SHA256          string `json:"sha256"`
}

type Store struct {
	Root        string
	SessionsDir string
	degraded    bool
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("chmod state directory: %w", err)
	}
	if err := os.Chmod(filepath.Join(root, "sessions"), 0o700); err != nil {
		return nil, fmt.Errorf("chmod sessions directory: %w", err)
	}
	return &Store{Root: root, SessionsDir: filepath.Join(root, "sessions")}, nil
}

func (s *Store) PersistenceDegraded() bool { return s.degraded }

func (s *Store) markError(err error) error { s.degraded = true; return err }

func (s *Store) atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".roaminal-*")
	if err != nil {
		return s.markError(err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return s.markError(err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return s.markError(err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return s.markError(err)
	}
	if err := tmp.Close(); err != nil {
		return s.markError(err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return s.markError(err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return s.markError(err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return s.markError(err)
	}
	return nil
}

func encodeJSON(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func (s *Store) SaveAuth(file AuthFile) error {
	file.FormatVersion = FormatVersion
	data, err := encodeJSON(file)
	if err != nil {
		return s.markError(err)
	}
	return s.atomicWrite(filepath.Join(s.Root, "auth-sessions.json"), append(data, '\n'))
}

func decodeStrict(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func (s *Store) LoadAuth() (AuthFile, error) {
	path := filepath.Join(s.Root, "auth-sessions.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AuthFile{FormatVersion: FormatVersion, Sessions: []AuthSession{}}, nil
	}
	if err != nil {
		return AuthFile{}, err
	}
	var file AuthFile
	if err := decodeStrict(data, &file); err != nil || file.FormatVersion != FormatVersion {
		_ = s.quarantine(path, "corrupt")
		return AuthFile{FormatVersion: FormatVersion, Sessions: []AuthSession{}}, nil
	}
	for _, session := range file.Sessions {
		if !uuidPattern.MatchString(session.ID) {
			return AuthFile{}, errors.New("invalid auth session id")
		}
	}
	return file, nil
}

func (s *Store) quarantine(path, suffix string) error {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return os.Rename(path, path+"."+suffix+"."+stamp)
}

func (s *Store) SessionPath(id string) string  { return filepath.Join(s.SessionsDir, id+".json") }
func (s *Store) SnapshotPath(id string) string { return filepath.Join(s.SessionsDir, id+".snapshot") }

func (s *Store) SaveSession(meta SessionMeta) error {
	meta.FormatVersion = FormatVersion
	data, err := encodeJSON(meta)
	if err != nil {
		return s.markError(err)
	}
	return s.atomicWrite(s.SessionPath(meta.ID), append(data, '\n'))
}

func (s *Store) DeleteSession(id string) error {
	for _, path := range []string{s.SessionPath(id), s.SnapshotPath(id)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return s.markError(err)
		}
	}
	return nil
}

func (s *Store) LoadSession(id string) (SessionMeta, error) {
	data, err := os.ReadFile(s.SessionPath(id))
	if err != nil {
		return SessionMeta{}, err
	}
	var meta SessionMeta
	if err := decodeStrict(data, &meta); err != nil || meta.FormatVersion != FormatVersion || !uuidPattern.MatchString(meta.ID) {
		_ = s.quarantine(s.SessionPath(id), "corrupt")
		_ = s.quarantine(s.SnapshotPath(id), "corrupt")
		return SessionMeta{}, fmt.Errorf("invalid session metadata %s", id)
	}
	return meta, nil
}

func (s *Store) ListSessions() ([]SessionMeta, error) {
	entries, err := os.ReadDir(s.SessionsDir)
	if err != nil {
		return nil, err
	}
	result := make([]SessionMeta, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		meta, err := s.LoadSession(id)
		if err != nil {
			continue
		}
		result = append(result, meta)
	}
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].CreatedAt.Before(result[j-1].CreatedAt); j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result, nil
}

func EncodeSnapshot(header SnapshotHeader, payload []byte) ([]byte, error) {
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
	if !strings.EqualFold(header.SHA256, hex.EncodeToString(digest[:])) {
		return SnapshotHeader{}, nil, errors.New("snapshot checksum mismatch")
	}
	if header.Cols < 1 || header.Rows < 1 || header.ScrollbackLines < 0 || header.ThroughSequence == "" {
		return SnapshotHeader{}, nil, errors.New("invalid snapshot dimensions")
	}
	return header, parts[2], nil
}

func (s *Store) SaveSnapshot(id string, header SnapshotHeader, payload []byte) error {
	data, err := EncodeSnapshot(header, payload)
	if err != nil {
		return s.markError(err)
	}
	return s.atomicWrite(s.SnapshotPath(id), data)
}

func (s *Store) LoadSnapshot(id string) (SnapshotHeader, []byte, error) {
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

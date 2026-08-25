package agent

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

// Agent state has its own component schema; the enclosing persistence schema
// version does not change the hook/telemetry record format.
const agentStoreSchemaVersion = 1

type fileState struct {
	FormatVersion int                       `json:"formatVersion"`
	Endpoints     map[string]EndpointRecord `json:"endpoints"`
}

type Store struct {
	path      string
	mu        sync.Mutex
	state     fileState
	available bool
	err       error
}

func OpenStore(root string) *Store {
	store := &Store{path: filepath.Join(root, "agent-endpoints.json"), state: fileState{FormatVersion: agentStoreSchemaVersion, Endpoints: map[string]EndpointRecord{}}}
	info, err := os.Lstat(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store
	}
	if err != nil {
		store.err = err
		return store
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		store.err = errors.New("agent store permissions are unsafe")
		return store
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		store.err = err
		return store
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&store.state)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if err == io.EOF {
			err = nil
		} else if err == nil {
			err = errors.New("agent store contains multiple JSON values")
		}
	}
	if err != nil || store.state.FormatVersion != agentStoreSchemaVersion || store.state.Endpoints == nil {
		if err == nil {
			err = errors.New("unsupported agent store format")
		}
		store.err = err
		return store
	}
	return store
}

func (s *Store) Available() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.err == nil }
func (s *Store) Err() error      { s.mu.Lock(); defer s.mu.Unlock(); return s.err }

func (s *Store) Snapshot() map[string]EndpointRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]EndpointRecord, len(s.state.Endpoints))
	for key, value := range s.state.Endpoints {
		result[key] = cloneRecord(value)
	}
	return result
}

func (s *Store) Get(key string) (EndpointRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.state.Endpoints[key]
	return cloneRecord(value), ok
}

func (s *Store) Update(key string, fn func(*EndpointRecord) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	value, existed := s.state.Endpoints[key]
	previous := cloneRecord(value)
	if value.Targets == nil {
		value.Targets = map[string]TargetState{}
	}
	if err := fn(&value); err != nil {
		return err
	}
	s.state.Endpoints[key] = value
	if err := s.saveLocked(); err != nil {
		if existed {
			s.state.Endpoints[key] = previous
		} else {
			delete(s.state.Endpoints, key)
		}
		return err
	}
	return nil
}

func (s *Store) FindToken(token string, now time.Time) (string, EndpointRecord, bool) {
	hash := tokenHash(token)
	if len(hash) != 64 {
		return "", EndpointRecord{}, false
	}
	digest, err := hex.DecodeString(hash)
	if err != nil {
		return "", EndpointRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, record := range s.state.Endpoints {
		for _, candidate := range []string{record.ActiveTokenHash, record.PendingTokenHash} {
			if equalHash(digest, candidate) {
				return key, cloneRecord(record), true
			}
		}
		if !expired(record.PreviousTokenExpiresAt, now) && equalHash(digest, record.PreviousTokenHash) {
			return key, cloneRecord(record), true
		}
	}
	return "", EndpointRecord{}, false
}

func (s *Store) saveLocked() error {
	s.state.FormatVersion = agentStoreSchemaVersion
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".agent-endpoints-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func tokenHash(token string) string {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func equalHash(raw []byte, encoded string) bool {
	candidate, err := hex.DecodeString(encoded)
	return err == nil && len(candidate) == len(raw) && subtle.ConstantTimeCompare(raw, candidate) == 1
}

func expired(value string, now time.Time) bool {
	if value == "" {
		return true
	}
	when, err := time.Parse(time.RFC3339Nano, value)
	return err != nil || !now.Before(when)
}

func cloneRecord(value EndpointRecord) EndpointRecord {
	if value.Aliases != nil {
		value.Aliases = append([]string(nil), value.Aliases...)
	}
	if value.Targets != nil {
		targets := value.Targets
		value.Targets = make(map[string]TargetState, len(targets))
		for key, state := range targets {
			value.Targets[key] = state
		}
	}
	return value
}

var _ ports.AgentRepository = (*Store)(nil)

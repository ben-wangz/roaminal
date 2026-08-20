package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
)

func (s *Service) Entries(ctx context.Context, id, relative, revision, cursor string, limit int) (DirectoryResult, error) {
	root, err := s.rootForRevision(ctx, id, revision)
	if err != nil {
		return DirectoryResult{}, err
	}
	clean, err := ValidateRelativePath(relative)
	if err != nil {
		return DirectoryResult{}, err
	}
	if limit == 0 {
		limit = defaultDirectoryLimit
	}
	if limit < 1 || limit > maxDirectoryLimit {
		return DirectoryResult{}, ErrInvalidPath
	}
	key := snapshotKey(id, root.Revision, clean)
	if cursor != "" {
		snapshot, offset, cursorErr := s.snapshotFromCursor(cursor, key)
		if cursorErr != nil {
			return DirectoryResult{}, cursorErr
		}
		return pageResult(id, root.Revision, clean, snapshot, offset, limit), nil
	}
	result, runErr := s.executor.RunRemote(ctx, id, connection.RemoteCommand{
		Script:      directoryScript,
		Args:        []string{root.AbsolutePath, clean},
		OutputLimit: maxDirectoryOutput,
		Timeout:     5 * time.Second,
	})
	if runErr != nil {
		return DirectoryResult{}, mapListingError(runErr)
	}
	raw, parseErr := parseDirectory(result.Output)
	if parseErr != nil {
		return DirectoryResult{}, parseErr
	}
	entries := make([]Entry, 0, len(raw))
	for _, value := range raw {
		entries = append(entries, makeEntry(root, clean, value))
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "directory"
		}
		return entries[i].Name < entries[j].Name
	})
	snapshot := s.saveSnapshot(key, entries)
	return pageResult(id, root.Revision, clean, snapshot, 0, limit), nil
}

func (s *Service) Stat(ctx context.Context, id, relative, revision string) (Entry, RootContext, error) {
	root, err := s.rootForRevision(ctx, id, revision)
	if err != nil {
		return Entry{}, RootContext{}, err
	}
	clean, err := ValidateRelativePath(relative)
	if err != nil {
		return Entry{}, RootContext{}, err
	}
	result, runErr := s.executor.RunRemote(ctx, id, connection.RemoteCommand{
		Script:      statScript,
		Args:        []string{root.AbsolutePath, clean},
		OutputLimit: 32 << 10,
		Timeout:     5 * time.Second,
	})
	if runErr != nil {
		return Entry{}, RootContext{}, mapStatError(runErr)
	}
	raw, parseErr := parseDirectory(result.Output)
	if parseErr != nil || len(raw) != 1 {
		return Entry{}, RootContext{}, ErrProtocol
	}
	return makeEntry(root, clean, raw[0]), root, nil
}

func makeEntry(root RootContext, parent string, raw rawEntry) Entry {
	relative := JoinRelative(parent, raw.Name)
	if parent == "." && raw.Name == "." {
		relative = "."
	}
	return Entry{
		Name:         raw.Name,
		RelativePath: relative,
		AbsolutePath: path.Join(root.AbsolutePath, relative),
		Type:         raw.Type,
		Size:         raw.Size,
		ModifiedAt:   raw.ModifiedAt,
		Mode:         raw.Mode,
		Symlink:      raw.Symlink,
	}
}

func snapshotKey(id, revision, relative string) string {
	return id + "\x00" + revision + "\x00" + relative
}

func (s *Service) saveSnapshot(key string, entries []Entry) DirectorySnapshot {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		copy(token[:], []byte(fmt.Sprintf("%016x", s.now().UnixNano())))
	}
	snapshot := DirectorySnapshot{
		ID:        base64.RawURLEncoding.EncodeToString(token[:]),
		Key:       key,
		Entries:   entries,
		ExpiresAt: s.now().Add(directorySnapshotTTL),
	}
	s.mu.Lock()
	for id, value := range s.snapshots {
		if !s.now().Before(value.ExpiresAt) {
			delete(s.snapshots, id)
		}
	}
	s.snapshots[snapshot.ID] = snapshot
	s.mu.Unlock()
	return snapshot
}

func (s *Service) snapshotFromCursor(cursor, key string) (DirectorySnapshot, int, error) {
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return DirectorySnapshot{}, 0, ErrInvalidCursor
	}
	parts := strings.Split(string(data), "\x00")
	if len(parts) != 2 {
		return DirectorySnapshot{}, 0, ErrInvalidCursor
	}
	offset, err := parseNonNegative(parts[1])
	if err != nil {
		return DirectorySnapshot{}, 0, ErrInvalidCursor
	}
	s.mu.Lock()
	snapshot, ok := s.snapshots[parts[0]]
	if ok && (!s.now().Before(snapshot.ExpiresAt) || snapshot.Key != key) {
		delete(s.snapshots, parts[0])
		ok = false
	}
	s.mu.Unlock()
	if !ok || offset > len(snapshot.Entries) {
		return DirectorySnapshot{}, 0, ErrInvalidCursor
	}
	return snapshot, offset, nil
}

func pageResult(id, revision, clean string, snapshot DirectorySnapshot, offset, limit int) DirectoryResult {
	end := offset + limit
	if end > len(snapshot.Entries) {
		end = len(snapshot.Entries)
	}
	entries := append([]Entry(nil), snapshot.Entries[offset:end]...)
	var next *string
	if end < len(snapshot.Entries) {
		value := base64.RawURLEncoding.EncodeToString([]byte(snapshot.ID + "\x00" + fmt.Sprintf("%d", end)))
		next = &value
	}
	return DirectoryResult{ConnectionInstanceID: id, RootRevision: revision, Path: clean, Entries: entries, NextCursor: next}
}

func parseNonNegative(value string) (int, error) {
	if value == "" {
		return 0, ErrInvalidCursor
	}
	parsed := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, ErrInvalidCursor
		}
		next := parsed*10 + int(char-'0')
		if next < parsed {
			return 0, ErrInvalidCursor
		}
		parsed = next
	}
	return parsed, nil
}

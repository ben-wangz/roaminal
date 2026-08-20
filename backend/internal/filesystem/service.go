package filesystem

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
)

const (
	rootProbeTimeout      = 2 * time.Second
	rootRequestTimeout    = 5 * time.Second
	rootProbeRetryDelay   = 100 * time.Millisecond
	rootFailureCacheTTL   = time.Second
	directorySnapshotTTL  = 2 * time.Second
	defaultDirectoryLimit = 200
	maxDirectoryLimit     = 500
	maxContentStream      = 512 << 20
)

type RemoteExecutor interface {
	Summaries() []connection.Summary
	RunRemote(context.Context, string, connection.RemoteCommand) (connection.RemoteResult, error)
	OpenRemote(context.Context, string, connection.RemoteCommand) (io.ReadCloser, error)
	RemoteTransferInfo(string) (connection.RemoteTransferInfo, error)
}

type Service struct {
	executor RemoteExecutor
	options  *connectionoptions.Store
	now      func() time.Time

	mu        sync.Mutex
	failures  map[string]time.Time
	snapshots map[string]DirectorySnapshot
	uploads   map[string]*uploadJob
	transfers map[string]transferCapability
}

func New(executor RemoteExecutor, options *connectionoptions.Store) *Service {
	return &Service{
		executor:  executor,
		options:   options,
		now:       time.Now,
		failures:  make(map[string]time.Time),
		snapshots: make(map[string]DirectorySnapshot),
		uploads:   make(map[string]*uploadJob),
		transfers: make(map[string]transferCapability),
	}
}

func (s *Service) Root(ctx context.Context, id string) (RootContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, rootRequestTimeout)
	defer cancel()
	summary, err := s.summary(id)
	if err != nil {
		return RootContext{}, err
	}
	if summary.Type != "ssh" {
		return RootContext{}, ErrUnsupported
	}
	if summary.Lifecycle != "live" {
		return RootContext{}, ErrNoTransport
	}
	if summary.SourceHostAlias == nil || strings.TrimSpace(*summary.SourceHostAlias) == "" {
		return RootContext{}, ErrNoTransport
	}
	alias := *summary.SourceHostAlias
	if summary.TmuxEnabled && summary.TmuxSessionName != "" && !s.tmuxFailureCached(id) {
		for attempt := 0; attempt < 2; attempt++ {
			probeCtx, probeCancel := context.WithTimeout(requestCtx, rootProbeTimeout)
			result, probeErr := s.executor.RunRemote(probeCtx, id, connection.RemoteCommand{
				Script:      tmuxRootScript,
				Args:        []string{summary.TmuxSessionName},
				OutputLimit: 16 << 10,
				Timeout:     rootProbeTimeout,
			})
			probeCancel()
			if probeErr == nil {
				if absolute, parseErr := parseRootOutput(result.Output); parseErr == nil {
					return s.makeRoot(id, absolute, "tmux", "current", summary.TmuxSessionName), nil
				}
			}
			if attempt == 0 {
				select {
				case <-time.After(rootProbeRetryDelay):
				case <-requestCtx.Done():
					return RootContext{}, mapRemoteError(requestCtx.Err())
				}
			}
		}
		s.rememberTmuxFailure(id)
	}

	pwd := connectionoptions.DefaultPwd
	if s.options != nil {
		if collection, loadErr := s.options.Load(map[string]bool{alias: true}); loadErr == nil {
			if option, ok := collection.Options[alias]; ok && option.Pwd != "" {
				pwd = option.Pwd
			}
		}
	}
	configuredCtx, configuredCancel := context.WithTimeout(requestCtx, rootProbeTimeout)
	result, configuredErr := s.executor.RunRemote(configuredCtx, id, connection.RemoteCommand{
		Script:      configuredRootScript,
		Args:        []string{pwd},
		OutputLimit: 16 << 10,
		Timeout:     rootProbeTimeout,
	})
	configuredCancel()
	if configuredErr == nil {
		if absolute, parseErr := parseRootOutput(result.Output); parseErr == nil {
			return s.makeRoot(id, absolute, "configured", "fallback", summary.TmuxSessionName), nil
		}
	}
	if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(configuredErr, context.DeadlineExceeded) {
		return RootContext{}, ErrTimeout
	}
	if mapped := mapRemoteError(configuredErr); mapped != configuredErr {
		return RootContext{}, mapped
	}
	return RootContext{}, ErrRootUnavailable
}

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

func (s *Service) OpenContent(ctx context.Context, id, relative, revision string, start, length int64) (ContentStream, error) {
	root, err := s.rootForRevision(ctx, id, revision)
	if err != nil {
		return ContentStream{}, err
	}
	clean, err := ValidateRelativePath(relative)
	if err != nil {
		return ContentStream{}, err
	}
	entry, err := s.statAtRoot(ctx, id, root, clean)
	if err != nil {
		return ContentStream{}, err
	}
	if entry.Type != "file" || entry.Symlink || entry.Size == nil {
		return ContentStream{}, ErrContentUnavailable
	}
	total := *entry.Size
	if start < 0 || length < 0 || start > total || length > total-start {
		return ContentStream{}, ErrContentUnavailable
	}
	if length > maxContentStream {
		return ContentStream{}, ErrContentTooLarge
	}
	reader, openErr := s.executor.OpenRemote(ctx, id, connection.RemoteCommand{
		Script:  contentScript,
		Args:    []string{root.AbsolutePath, clean, fmt.Sprintf("%d", start), fmt.Sprintf("%d", length)},
		Timeout: 15 * time.Minute,
	})
	if openErr != nil {
		return ContentStream{}, mapRemoteError(openErr)
	}
	end := start
	if length > 0 {
		end += length - 1
	}
	return ContentStream{Reader: reader, Entry: entry, Root: root, Start: start, End: end, TotalSize: total, ContentLength: length}, nil
}

func (s *Service) statAtRoot(ctx context.Context, id string, root RootContext, clean string) (Entry, error) {
	result, runErr := s.executor.RunRemote(ctx, id, connection.RemoteCommand{
		Script:      statScript,
		Args:        []string{root.AbsolutePath, clean},
		OutputLimit: 32 << 10,
		Timeout:     5 * time.Second,
	})
	if runErr != nil {
		return Entry{}, mapStatError(runErr)
	}
	raw, parseErr := parseDirectory(result.Output)
	if parseErr != nil || len(raw) != 1 {
		return Entry{}, ErrProtocol
	}
	return makeEntry(root, clean, raw[0]), nil
}

func (s *Service) rootForRevision(ctx context.Context, id, revision string) (RootContext, error) {
	root, err := s.Root(ctx, id)
	if err != nil {
		return RootContext{}, err
	}
	if revision != "" && revision != root.Revision {
		return RootContext{}, &RootChangedError{Root: root}
	}
	return root, nil
}

func (s *Service) summary(id string) (connection.Summary, error) {
	if s.executor == nil {
		return connection.Summary{}, ErrNoTransport
	}
	for _, summary := range s.executor.Summaries() {
		if summary.ID == id || summary.ConnectionInstanceID == id {
			return summary, nil
		}
	}
	return connection.Summary{}, ErrInstanceNotFound
}

func (s *Service) makeRoot(id, absolute, source, status, sessionName string) RootContext {
	digest := sha256.Sum256([]byte(source + "\x00" + absolute + "\x00" + sessionName))
	return RootContext{
		ConnectionInstanceID: id,
		AbsolutePath:         absolute,
		RelativePath:         ".",
		Source:               source,
		Status:               status,
		Revision:             base64.RawURLEncoding.EncodeToString(digest[:16]),
		ResolvedAt:           s.now().UTC(),
	}
}

func (s *Service) tmuxFailureCached(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.failures[id]
	if !ok || !s.now().Before(until) {
		if ok {
			delete(s.failures, id)
		}
		return false
	}
	return true
}

func (s *Service) rememberTmuxFailure(id string) {
	s.mu.Lock()
	s.failures[id] = s.now().Add(rootFailureCacheTTL)
	s.mu.Unlock()
}

func parseRootOutput(data []byte) (string, error) {
	parts := strings.Split(string(data), "\x00")
	if len(parts) != 3 || parts[0] != rootBeginMarker || parts[1] == "" || !strings.HasPrefix(parts[1], "/") || strings.ContainsAny(parts[1], "\r\n\x00") {
		return "", ErrProtocol
	}
	return path.Clean(parts[1]), nil
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

func mapRemoteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, connection.ErrRemoteInstanceNotFound) {
		return ErrInstanceNotFound
	}
	if errors.Is(err, connection.ErrRemoteNoTransport) || errors.Is(err, connection.ErrTransportUnavailable) {
		return ErrNoTransport
	}
	return err
}

func mapListingError(err error) error {
	if mapped := mapRemoteError(err); mapped != err {
		return mapped
	}
	return &RemoteOperationError{Operation: ErrListingFailed.Error(), Err: err}
}

func mapStatError(err error) error {
	if mapped := mapRemoteError(err); mapped != err {
		return mapped
	}
	return &RemoteOperationError{Operation: ErrNotFound.Error(), Err: err}
}

const rootBeginMarker = "ROAMINAL_FILESYSTEM_ROOT_V1"

const tmuxRootScript = `set -eu
session_name=$1
if ! command -v tmux >/dev/null 2>&1; then exit 20; fi
tmux has-session -t "=$session_name"
path=$(tmux display-message -p -t "=$session_name" '#{pane_current_path}')
case "$path" in /*) ;; *) exit 21;; esac
path=$(cd -- "$path" && pwd -P)
test -d "$path"
printf '%s\0%s\0' 'ROAMINAL_FILESYSTEM_ROOT_V1' "$path"
`

const configuredRootScript = `set -eu
value=$1
case "$value" in
  '$HOME') candidate=$HOME ;;
  '~') candidate=$HOME ;;
  '$HOME/'*) candidate=$HOME/${value#'$HOME/'} ;;
  '~/'*) candidate=$HOME/${value#'~/'} ;;
  /*) candidate=$value ;;
  *) exit 22 ;;
esac
path=$(cd -- "$candidate" && pwd -P)
test -d "$path"
printf '%s\0%s\0' 'ROAMINAL_FILESYSTEM_ROOT_V1' "$path"
`

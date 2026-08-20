package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"path"
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

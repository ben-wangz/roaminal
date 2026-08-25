package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
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

type RemoteExecutor = ports.RemoteExecutor

type Service struct {
	remote  ports.RemoteFileSystem
	options *connectionoptions.Store
	clock   ports.Clock
	random  ports.RandomSource
	now     func() time.Time

	mu          sync.Mutex
	failures    map[string]time.Time
	snapshots   map[string]DirectorySnapshot
	uploads     map[string]*uploadJob
	transfers   map[string]transferCapability
	uploadRepo  ports.UploadRepository
	stagingRoot string
}

type Dependencies struct {
	Clock  ports.Clock
	Random ports.RandomSource
}

func New(executor RemoteExecutor, options *connectionoptions.Store, dependencies ...Dependencies) *Service {
	return NewWithRemote(newRemoteFileSystemAdapter(executor), options, nil, "", dependencies...)
}

func NewWithRepositories(executor RemoteExecutor, options *connectionoptions.Store, uploadRepo ports.UploadRepository, stateRoot string, dependencies ...Dependencies) *Service {
	return NewWithRemote(newRemoteFileSystemAdapter(executor), options, uploadRepo, stateRoot, dependencies...)
}

func NewWithRemote(remote ports.RemoteFileSystem, options *connectionoptions.Store, uploadRepo ports.UploadRepository, stateRoot string, dependencies ...Dependencies) *Service {
	deps := Dependencies{Clock: systemclock.System{}, Random: random.CryptoSource{}}
	if len(dependencies) > 0 {
		if dependencies[0].Clock != nil {
			deps.Clock = dependencies[0].Clock
		}
		if dependencies[0].Random != nil {
			deps.Random = dependencies[0].Random
		}
	}
	if uploadRepo == nil {
		uploadRepo = newMemoryUploadRepository()
	}
	stagingRoot := ""
	if stateRoot != "" {
		stagingRoot = filepath.Join(stateRoot, "uploads")
	}
	return &Service{
		remote:      remote,
		options:     options,
		clock:       deps.Clock,
		random:      deps.Random,
		now:         deps.Clock.Now,
		failures:    make(map[string]time.Time),
		snapshots:   make(map[string]DirectorySnapshot),
		uploads:     make(map[string]*uploadJob),
		transfers:   make(map[string]transferCapability),
		uploadRepo:  uploadRepo,
		stagingRoot: stagingRoot,
	}
}

func (s *Service) Root(ctx context.Context, id string) (RootContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, rootRequestTimeout)
	defer cancel()
	if s.remote == nil {
		return RootContext{}, ErrNoTransport
	}
	summary, err := s.remote.ConnectionInstance(id)
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
			result, probeErr := s.remote.ProbeTmuxRoot(probeCtx, id, summary.TmuxSessionName)
			probeCancel()
			if probeErr == nil {
				return s.makeRoot(id, result.AbsolutePath, result.Source, result.Status, summary.TmuxSessionName), nil
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
	result, configuredErr := s.remote.ProbeConfiguredRoot(configuredCtx, id, pwd)
	configuredCancel()
	if configuredErr == nil {
		return s.makeRoot(id, result.AbsolutePath, result.Source, result.Status, summary.TmuxSessionName), nil
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
	if errors.Is(err, ports.ErrRemoteInstanceNotFound) {
		return ErrInstanceNotFound
	}
	if errors.Is(err, ports.ErrRemoteNoTransport) {
		return ErrNoTransport
	}
	if errors.Is(err, ports.ErrTransportUnavailable) {
		return ErrTransportUnavailable
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

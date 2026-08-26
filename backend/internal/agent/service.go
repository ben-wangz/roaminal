package agent

import (
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

type Service struct {
	cfg            config.Config
	terms          ConnectionService
	store          ports.AgentRepository
	mu             sync.Mutex
	bindings       map[string]Target
	operations     map[string]*Initialization
	endpointOps    map[string]string
	endpointLock   map[string]*sync.Mutex
	rate           map[string]eventRate
	runtime        map[string]TargetState
	runtimeTargets map[string]string
	completedTools map[string]map[string]time.Time
	eventIDs       map[string]map[string]time.Time
	eventLocks     map[string]*sync.Mutex
	endpointCache  map[string]endpointCacheEntry
	clock          ports.Clock
	ids            ports.IDGenerator
	random         ports.RandomSource
	messages       ports.MessageAppender
}

type Dependencies struct {
	Clock    ports.Clock
	IDs      ports.IDGenerator
	Random   ports.RandomSource
	Messages ports.MessageAppender
}

type eventRate struct {
	LastTokens time.Time
	Tokens     float64
}

type endpointCacheEntry struct {
	Alias       string
	SourceState string
	Key         string
	ExpiresAt   time.Time
}

func NewWithRepository(cfg config.Config, repository ports.AgentRepository, terms ConnectionService, dependencies ...Dependencies) *Service {
	deps := Dependencies{Clock: systemclock.System{}, Random: random.CryptoSource{}}
	if len(dependencies) > 0 {
		if dependencies[0].Clock != nil {
			deps.Clock = dependencies[0].Clock
		}
		if dependencies[0].Random != nil {
			deps.Random = dependencies[0].Random
		}
		if dependencies[0].IDs != nil {
			deps.IDs = dependencies[0].IDs
		}
		if dependencies[0].Messages != nil {
			deps.Messages = dependencies[0].Messages
		}
	}
	if deps.IDs == nil {
		deps.IDs = identity.UUIDGenerator{Random: deps.Random}
	}
	return &Service{
		cfg: cfg, terms: terms, store: repository, bindings: map[string]Target{},
		operations: map[string]*Initialization{}, endpointOps: map[string]string{}, endpointLock: map[string]*sync.Mutex{},
		rate: map[string]eventRate{}, runtime: map[string]TargetState{}, runtimeTargets: map[string]string{}, completedTools: map[string]map[string]time.Time{}, eventIDs: map[string]map[string]time.Time{}, eventLocks: map[string]*sync.Mutex{}, endpointCache: map[string]endpointCacheEntry{},
		clock: deps.Clock, ids: deps.IDs, random: deps.Random, messages: deps.Messages,
	}
}

func (s *Service) eventLockFor(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eventLocks == nil {
		s.eventLocks = map[string]*sync.Mutex{}
	}
	if lock := s.eventLocks[key]; lock != nil {
		return lock
	}
	lock := &sync.Mutex{}
	s.eventLocks[key] = lock
	return lock
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock.Now()
	}
	return systemclock.System{}.Now()
}

func (s *Service) since(start time.Time) time.Duration {
	if s.clock != nil {
		return s.clock.Since(start)
	}
	return systemclock.System{}.Since(start)
}

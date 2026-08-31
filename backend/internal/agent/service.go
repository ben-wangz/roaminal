package agent

import (
	"context"
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/identity"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

type Service struct {
	cfg          config.Config
	terms        ConnectionService
	store        ports.AgentRepository
	mu           sync.Mutex
	bindings     map[string]Target
	operations   map[string]*Initialization
	endpointOps  map[string]string
	endpointLock map[string]*sync.Mutex
	clock        ports.Clock
	ids          ports.IDGenerator
	random       ports.RandomSource
	messages     ports.MessageAppender
	syncInterval time.Duration
	syncCancel   context.CancelFunc
	syncWait     sync.WaitGroup
}

type Dependencies struct {
	Clock        ports.Clock
	IDs          ports.IDGenerator
	Random       ports.RandomSource
	Messages     ports.MessageAppender
	SyncInterval time.Duration
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
	if deps.SyncInterval <= 0 {
		deps.SyncInterval = time.Minute
	}
	return &Service{
		cfg: cfg, terms: terms, store: repository, bindings: map[string]Target{},
		operations: map[string]*Initialization{}, endpointOps: map[string]string{}, endpointLock: map[string]*sync.Mutex{},
		clock: deps.Clock, ids: deps.IDs, random: deps.Random, messages: deps.Messages, syncInterval: deps.SyncInterval,
	}
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

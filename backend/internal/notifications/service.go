package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var (
	ErrUnavailable                = errors.New("web push is unavailable")
	ErrStoreUnavailable           = errors.New("web push subscription store unavailable")
	ErrInvalidSubscription        = errors.New("web push subscription is invalid")
	ErrInvalidPreference          = errors.New("notification preference is invalid")
	ErrPreferenceStoreUnavailable = errors.New("notification preference store unavailable")
)

const (
	defaultQueueSize     = 64
	defaultSendTimeout   = 10 * time.Second
	defaultRetryAttempts = 3
	defaultRetryDelay    = 250 * time.Millisecond
)

// Sender is the delivery boundary. The implementation owns Web Push
// encryption and provider-specific HTTP details; the service owns retries and
// subscription lifecycle.
type Sender interface {
	Send(context.Context, []byte, domain.PushSubscriptionRecord) (SendOutcome, error)
}

type SendOutcome struct {
	StatusCode int
	Permanent  bool
	Retryable  bool
}

type Options struct {
	PublicKey            string
	PrivateKey           string
	Subject              string
	Sender               Sender
	PreferenceRepository ports.NotificationPreferenceRepository
	UserKey              string
	Clock                ports.Clock
	QueueSize            int
	RetryAttempts        int
	RetryDelay           time.Duration
	SendTimeout          time.Duration
}

type ConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"publicKey,omitempty"`
}

type SubscriptionInput struct {
	Endpoint  string
	AuthKey   string
	P256dhKey string
}

type SubscriptionResult struct {
	ID string `json:"subscriptionId"`
}

type Preference struct {
	ConnectionDefinitionID string `json:"connectionDefinitionId"`
	TmuxSessionName        string `json:"tmuxSessionName"`
	Enabled                bool   `json:"enabled"`
	RunningToRelax         bool   `json:"runningToRelax"`
	RunningToError         bool   `json:"runningToError"`
}

type PreferenceInput struct {
	ConnectionDefinitionID string
	TmuxSessionName        string
	Enabled                bool
	RunningToRelax         bool
	RunningToError         bool
}

type notificationJob struct {
	record  domain.MessageRecord
	payload []byte
}

type Service struct {
	repository  ports.PushSubscriptionRepository
	preferences ports.NotificationPreferenceRepository
	userKey     string
	ids         ports.IDGenerator
	clock       ports.Clock
	sender      Sender
	publicKey   string
	enabled     bool

	queue         chan notificationJob
	retryAttempts int
	retryDelay    time.Duration
	sendTimeout   time.Duration
	workers       sync.WaitGroup
	pending       sync.WaitGroup
	context       context.Context
	cancel        context.CancelFunc
	lifecycleMu   sync.RWMutex
	closed        bool
}

func New(repository ports.PushSubscriptionRepository, ids ports.IDGenerator, options Options) (*Service, error) {
	if options.Clock == nil {
		options.Clock = clock.System{}
	}
	if options.QueueSize <= 0 {
		options.QueueSize = defaultQueueSize
	}
	if options.RetryAttempts <= 0 {
		options.RetryAttempts = defaultRetryAttempts
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = defaultRetryDelay
	}
	if options.SendTimeout <= 0 {
		options.SendTimeout = defaultSendTimeout
	}

	publicKey := strings.TrimSpace(options.PublicKey)
	privateKey := strings.TrimSpace(options.PrivateKey)
	subject := strings.TrimSpace(options.Subject)
	hasPublicKey := publicKey != ""
	hasPrivateKey := privateKey != ""
	hasSubject := subject != ""
	configured := hasPublicKey || hasPrivateKey || hasSubject
	if configured && (!hasPublicKey || !hasPrivateKey || !hasSubject) {
		return nil, fmt.Errorf("%w: VAPID configuration is incomplete", ErrUnavailable)
	}
	sender := options.Sender
	if sender == nil && configured {
		var err error
		sender, err = newWebPushSender(publicKey, privateKey, subject)
		if err != nil {
			return nil, fmt.Errorf("%w: initialize sender: %v", ErrUnavailable, err)
		}
	}
	// A custom sender is useful for tests and alternate providers, but browser
	// registration still needs the public VAPID key to be advertised.
	enabled := sender != nil && hasPublicKey
	service := &Service{
		repository:    repository,
		preferences:   options.PreferenceRepository,
		userKey:       strings.TrimSpace(options.UserKey),
		ids:           ids,
		clock:         options.Clock,
		sender:        sender,
		publicKey:     publicKey,
		enabled:       enabled,
		queue:         make(chan notificationJob, options.QueueSize),
		retryAttempts: options.RetryAttempts,
		retryDelay:    options.RetryDelay,
		sendTimeout:   options.SendTimeout,
	}
	if enabled {
		service.context, service.cancel = context.WithCancel(context.Background())
		service.workers.Add(1)
		go service.worker()
	}
	return service, nil
}

func (s *Service) Configuration() ConfigResponse {
	if s == nil {
		return ConfigResponse{}
	}
	return ConfigResponse{Enabled: s.enabled, PublicKey: s.publicKey}
}

func (s *Service) Register(ctx context.Context, authSessionID string, input SubscriptionInput) (SubscriptionResult, error) {
	if s == nil || !s.enabled || s.repository == nil || s.ids == nil {
		return SubscriptionResult{}, ErrUnavailable
	}
	authSessionID = strings.TrimSpace(authSessionID)
	if authSessionID == "" {
		return SubscriptionResult{}, ErrInvalidSubscription
	}
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.AuthKey = strings.TrimSpace(input.AuthKey)
	input.P256dhKey = strings.TrimSpace(input.P256dhKey)
	if err := validateSubscription(input); err != nil {
		return SubscriptionResult{}, err
	}
	id, err := s.ids.NewID()
	if err != nil || strings.TrimSpace(id) == "" {
		if err == nil {
			err = errors.New("empty subscription id")
		}
		return SubscriptionResult{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	now := s.clock.Now().UTC()
	record, err := s.repository.UpsertPushSubscription(ctx, domain.PushSubscriptionRecord{
		ID: id, AuthenticationSessionID: authSessionID, Endpoint: input.Endpoint,
		AuthKey: input.AuthKey, P256dhKey: input.P256dhKey, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return SubscriptionResult{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return SubscriptionResult{ID: record.ID}, nil
}

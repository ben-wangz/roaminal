package notifications

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var (
	ErrUnavailable          = errors.New("web push is unavailable")
	ErrStoreUnavailable     = errors.New("web push subscription store unavailable")
	ErrInvalidSubscription  = errors.New("web push subscription is invalid")
	ErrSubscriptionNotFound = errors.New("web push subscription not found")
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
	PublicKey     string
	PrivateKey    string
	Subject       string
	Sender        Sender
	Clock         ports.Clock
	QueueSize     int
	RetryAttempts int
	RetryDelay    time.Duration
	SendTimeout   time.Duration
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

type notificationJob struct {
	record  domain.MessageRecord
	payload []byte
}

type Service struct {
	repository ports.PushSubscriptionRepository
	ids        ports.IDGenerator
	clock      ports.Clock
	sender     Sender
	publicKey  string
	enabled    bool

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

func (s *Service) Delete(ctx context.Context, authSessionID, subscriptionID string) error {
	if s == nil || s.repository == nil {
		return ErrStoreUnavailable
	}
	authSessionID = strings.TrimSpace(authSessionID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if authSessionID == "" || subscriptionID == "" {
		return ErrInvalidSubscription
	}
	if _, err := s.repository.DeletePushSubscription(ctx, authSessionID, subscriptionID); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

func (s *Service) DeleteAll(ctx context.Context, authSessionID string) error {
	if s == nil || s.repository == nil {
		return ErrStoreUnavailable
	}
	authSessionID = strings.TrimSpace(authSessionID)
	if authSessionID == "" {
		return ErrInvalidSubscription
	}
	if _, err := s.repository.DeletePushSubscriptionsForAuthSession(ctx, authSessionID); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

func (s *Service) DeleteByID(ctx context.Context, subscriptionID string) error {
	if s == nil || s.repository == nil {
		return ErrStoreUnavailable
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ErrInvalidSubscription
	}
	if _, err := s.repository.DeletePushSubscriptionByID(ctx, subscriptionID); err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

// Prune removes subscriptions belonging to expired authentication sessions.
// It is intentionally best effort at startup: notification availability must
// not prevent the terminal service from starting.
func (s *Service) Prune(ctx context.Context, active func(string) bool) error {
	if s == nil || s.repository == nil || active == nil {
		return nil
	}
	records, err := s.repository.ListPushSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	removed := 0
	for _, record := range records {
		if active(record.AuthenticationSessionID) {
			continue
		}
		if err := s.DeleteByID(ctx, record.ID); err != nil {
			return err
		}
		removed++
	}
	if removed > 0 {
		log.Printf("level=INFO event=web_push_subscriptions_pruned removed=%d", removed)
	}
	return nil
}

// Notify is deliberately non-blocking. Message persistence has already
// succeeded by the time this method is called.
func (s *Service) Notify(record domain.MessageRecord) {
	if s == nil || !eligible(record.Kind) || !s.enabled || s.repository == nil {
		return
	}
	payload, err := notificationPayload(record)
	if err != nil {
		log.Printf("level=INFO event=web_push_payload_failed error_type=%T", err)
		return
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	if s.closed {
		return
	}
	s.pending.Add(1)
	select {
	case s.queue <- notificationJob{record: record, payload: payload}:
	default:
		s.pending.Done()
		log.Printf("level=INFO event=web_push_queue_full")
	}
}

func (s *Service) worker() {
	defer s.workers.Done()
	for job := range s.queue {
		s.dispatch(job)
		s.pending.Done()
	}
}

func (s *Service) dispatch(job notificationJob) {
	started := s.clock.Now()
	records, err := s.repository.ListPushSubscriptions(s.context)
	if err != nil {
		log.Printf("level=INFO event=web_push_delivery_list_failed error_type=%T", err)
		return
	}
	delivered, failed, removed := 0, 0, 0
	for _, record := range records {
		outcome, sendErr := s.sendWithRetry(job.payload, record)
		if outcome.Permanent {
			if err := s.DeleteByID(s.context, record.ID); err != nil {
				log.Printf("level=INFO event=web_push_subscription_delete_failed error_type=%T", err)
			} else {
				removed++
			}
		}
		if sendErr != nil {
			failed++
			continue
		}
		delivered++
	}
	log.Printf("level=INFO event=web_push_delivery_completed subscriptions=%d delivered=%d failed=%d removed=%d duration_ms=%d", len(records), delivered, failed, removed, s.clock.Since(started).Milliseconds())
}

func (s *Service) sendWithRetry(payload []byte, record domain.PushSubscriptionRecord) (SendOutcome, error) {
	var lastOutcome SendOutcome
	var lastErr error
	for attempt := 1; attempt <= s.retryAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(s.context, s.sendTimeout)
		outcome, err := s.sender.Send(ctx, payload, record)
		cancel()
		lastOutcome, lastErr = outcome, err
		if err == nil && outcome.StatusCode >= 200 && outcome.StatusCode < 300 {
			return outcome, nil
		}
		if outcome.Permanent {
			return outcome, err
		}
		if !outcome.Retryable || attempt == s.retryAttempts {
			return outcome, errOrStatus(outcome, err)
		}
		if !wait(s.context, s.retryDelay*time.Duration(attempt)) {
			return outcome, context.Canceled
		}
	}
	return lastOutcome, errOrStatus(lastOutcome, lastErr)
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func errOrStatus(outcome SendOutcome, err error) error {
	if err != nil {
		return err
	}
	if outcome.StatusCode != 0 {
		return fmt.Errorf("push service returned HTTP %d", outcome.StatusCode)
	}
	return errors.New("push delivery failed")
}

func (s *Service) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan struct{})
	go func() {
		s.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	if !s.closed {
		s.closed = true
		if s.cancel != nil {
			s.cancel()
		}
		close(s.queue)
	}
	s.lifecycleMu.Unlock()
	s.workers.Wait()
	return nil
}

func eligible(kind string) bool {
	return kind == "codex_turn_completed" || kind == "codex_turn_failed"
}

type notificationPayloadData struct {
	MessageID string `json:"messageId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Severity  string `json:"severity"`
}

func notificationPayload(record domain.MessageRecord) ([]byte, error) {
	body := "Codex turn finished"
	if record.Kind == "codex_turn_failed" {
		body = "Codex turn failed"
	}
	return json.Marshal(notificationPayloadData{MessageID: record.MessageID, Title: "Roaminal", Body: body, Severity: record.Severity})
}

func validateSubscription(input SubscriptionInput) error {
	if len([]byte(input.Endpoint)) == 0 || len([]byte(input.Endpoint)) > 4096 {
		return ErrInvalidSubscription
	}
	u, err := url.Parse(input.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return ErrInvalidSubscription
	}
	if _, err := decodeSubscriptionKey(input.AuthKey, 16); err != nil {
		return ErrInvalidSubscription
	}
	if p256dh, err := decodeSubscriptionKey(input.P256dhKey, 65); err != nil {
		return ErrInvalidSubscription
	} else if x, y := elliptic.Unmarshal(elliptic.P256(), p256dh); x == nil || y == nil {
		return ErrInvalidSubscription
	}
	return nil
}

func decodeSubscriptionKey(value string, size int) ([]byte, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "=")
	var (
		decoded []byte
		err     error
	)
	if strings.ContainsAny(value, "+/") {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	} else {
		decoded, err = base64.RawURLEncoding.DecodeString(value)
	}
	if err != nil || len(decoded) != size {
		return nil, ErrInvalidSubscription
	}
	return decoded, nil
}

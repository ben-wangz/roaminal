package notifications

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestDisabledServiceDoesNotQueueNotifications(t *testing.T) {
	repo := &fakeRepository{}
	sender := &fakeSender{}
	service, err := New(repo, &fakeIDs{}, Options{Sender: sender})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if service.Configuration().Enabled {
		t.Fatal("service without VAPID public key must be disabled")
	}
	service.Notify(completedRecord())
	if sender.calls() != 0 {
		t.Fatal("disabled service sent a notification")
	}
}

func TestServiceRegistersAndSendsSafePayload(t *testing.T) {
	repo := &fakeRepository{}
	sender := &fakeSender{outcomes: []SendOutcome{{StatusCode: 201}}}
	service, err := New(repo, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com",
		Sender: sender, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if !service.Configuration().Enabled {
		t.Fatal("configured service is disabled")
	}
	input := validSubscriptionInput(t)
	registered, err := service.Register(context.Background(), "session-1", input)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if registered.ID == "" || len(repo.records) != 1 {
		t.Fatalf("unexpected registration: %+v records=%d", registered, len(repo.records))
	}
	service.Notify(completedRecord())
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	call := sender.lastCall()
	var payload map[string]any
	if err := json.Unmarshal(call.payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["messageId"] != "message-1" || payload["body"] != "Codex turn finished" || payload["severity"] != "success" {
		t.Fatalf("unexpected payload: %s", call.payload)
	}
	if string(call.payload) == input.Endpoint || string(call.payload) == input.AuthKey || string(call.payload) == input.P256dhKey {
		t.Fatal("payload contains subscription material")
	}
}

func TestServiceRetriesTransientAndRemovesExpiredSubscription(t *testing.T) {
	repo := &fakeRepository{}
	sender := &fakeSender{outcomes: []SendOutcome{
		{StatusCode: 503, Retryable: true},
		{StatusCode: 503, Retryable: true},
		{StatusCode: 201},
	}}
	service, err := New(repo, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com",
		Sender: sender, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	input := validSubscriptionInput(t)
	if _, err := service.Register(context.Background(), "session-1", input); err != nil {
		t.Fatal(err)
	}
	service.Notify(completedRecord())
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.calls() != 3 {
		t.Fatalf("expected three delivery attempts, got %d", sender.calls())
	}

	repo2 := &fakeRepository{}
	sender2 := &fakeSender{outcomes: []SendOutcome{{StatusCode: 410, Permanent: true}, {StatusCode: 201}}}
	service2, err := New(repo2, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com",
		Sender: sender2, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service2.Close()
	if _, err := service2.Register(context.Background(), "session-1", input); err != nil {
		t.Fatal(err)
	}
	service2.Notify(completedRecord())
	if err := service2.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo2.records) != 0 {
		t.Fatal("expired subscription was not removed")
	}
}

func TestServiceRejectsInvalidSubscription(t *testing.T) {
	service, err := New(&fakeRepository{}, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", Sender: &fakeSender{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	_, err = service.Register(context.Background(), "session-1", SubscriptionInput{Endpoint: "http://push.example.test", AuthKey: "bad", P256dhKey: "bad"})
	if !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("error=%v", err)
	}
}

func completedRecord() domain.MessageRecord {
	now := time.Now().UTC()
	return domain.MessageRecord{MessageDraft: domain.MessageDraft{
		MessageID: "message-1", Kind: "codex_turn_completed", Severity: "success", PresentationKey: "codex_turn_finished",
		OccurredAt: now, ReceivedAt: now,
	}, Sequence: 1}
}

func validSubscriptionInput(t *testing.T) SubscriptionInput {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	return SubscriptionInput{
		Endpoint: "https://push.example.test/send/token",
		AuthKey:  base64.RawURLEncoding.EncodeToString(auth), P256dhKey: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
	}
}

type fakeIDs struct{ next int }

func (g *fakeIDs) NewID() (string, error) {
	g.next++
	return "00000000-0000-4000-8000-00000000000" + string(rune('0'+g.next)), nil
}

type fakeRepository struct {
	mu      sync.Mutex
	records map[string]domain.PushSubscriptionRecord
}

func (r *fakeRepository) ListPushSubscriptions(context.Context) ([]domain.PushSubscriptionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domain.PushSubscriptionRecord, 0, len(r.records))
	for _, record := range r.records {
		result = append(result, record)
	}
	return result, nil
}

func (r *fakeRepository) UpsertPushSubscription(_ context.Context, record domain.PushSubscriptionRecord) (domain.PushSubscriptionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[string]domain.PushSubscriptionRecord)
	}
	for id, existing := range r.records {
		if existing.Endpoint == record.Endpoint {
			record.ID = id
			record.CreatedAt = existing.CreatedAt
			delete(r.records, id)
		}
	}
	r.records[record.ID] = record
	return record, nil
}

func (r *fakeRepository) DeletePushSubscription(_ context.Context, _, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.records[id]; !ok {
		return false, nil
	}
	delete(r.records, id)
	return true, nil
}

func (r *fakeRepository) DeletePushSubscriptionsForAuthSession(_ context.Context, session string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for id, record := range r.records {
		if record.AuthenticationSessionID == session {
			delete(r.records, id)
			count++
		}
	}
	return count, nil
}

func (r *fakeRepository) DeletePushSubscriptionByID(_ context.Context, id string) (bool, error) {
	return r.DeletePushSubscription(context.Background(), "", id)
}

type sendCall struct {
	payload []byte
	record  domain.PushSubscriptionRecord
}

type fakeSender struct {
	mu       sync.Mutex
	outcomes []SendOutcome
	callList []sendCall
}

func (s *fakeSender) Send(_ context.Context, payload []byte, record domain.PushSubscriptionRecord) (SendOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callList = append(s.callList, sendCall{payload: append([]byte(nil), payload...), record: record})
	if len(s.outcomes) == 0 {
		return SendOutcome{StatusCode: 201}, nil
	}
	index := len(s.callList) - 1
	if index >= len(s.outcomes) {
		index = len(s.outcomes) - 1
	}
	outcome := s.outcomes[index]
	if outcome.StatusCode >= 400 {
		return outcome, errors.New("fake send failure")
	}
	return outcome, nil
}

func (s *fakeSender) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.callList)
}

func (s *fakeSender) lastCall() sendCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callList[len(s.callList)-1]
}

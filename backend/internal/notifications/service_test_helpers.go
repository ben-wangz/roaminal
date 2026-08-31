package notifications

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func completedRecord() domain.MessageRecord {
	now := time.Now().UTC()
	return domain.MessageRecord{MessageDraft: domain.MessageDraft{
		MessageID: "message-1", Kind: "agent_state_transition", Severity: "success", PresentationKey: "agent_state_transition",
		OccurredAt: now, ReceivedAt: now, TmuxSessionName: "team", TmuxSessionID: "$0", TmuxSessionCreated: 1,
		EndpointKey: "endpoint-1", FallbackLabel: "Remote / tmux:team", AgentType: "codex",
		ConnectionDefinitionIDs: []string{"definition-1"}, AgentStateFrom: "running", AgentStateTo: "relax", AgentRuntimeID: "runtime-1", AgentStateIndex: 1,
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
	mu          sync.Mutex
	records     map[string]domain.PushSubscriptionRecord
	preferences map[string]domain.NotificationPreference
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

func (r *fakeRepository) ListNotificationPreferences(_ context.Context, userKey string) ([]domain.NotificationPreference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domain.NotificationPreference, 0, len(r.preferences))
	for _, preference := range r.preferences {
		if preference.UserKey == userKey {
			result = append(result, preference)
		}
	}
	return result, nil
}

func (r *fakeRepository) GetNotificationPreference(_ context.Context, userKey, definitionID, sessionName string) (domain.NotificationPreference, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	preference, ok := r.preferences[userKey+"\x00"+definitionID+"\x00"+sessionName]
	return preference, ok, nil
}

func (r *fakeRepository) UpsertNotificationPreference(_ context.Context, preference domain.NotificationPreference) (domain.NotificationPreference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.preferences == nil {
		r.preferences = make(map[string]domain.NotificationPreference)
	}
	r.preferences[preference.UserKey+"\x00"+preference.ConnectionDefinitionID+"\x00"+preference.TmuxSessionName] = preference
	return preference, nil
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

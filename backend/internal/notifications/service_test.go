package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
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
	record := completedRecord()
	record.ConnectionLabel = "pve-roaminal"
	service.Notify(record)
	if sender.calls() != 0 {
		t.Fatal("disabled service sent a notification")
	}
}

func TestServiceRegistersAndSendsSafePayload(t *testing.T) {
	repo := &fakeRepository{}
	sender := &fakeSender{outcomes: []SendOutcome{{StatusCode: 201}}}
	service, err := New(repo, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", SessionActive: alwaysActive,
		Sender: sender, PreferenceRepository: repo, UserKey: "test-user", RetryDelay: time.Millisecond,
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
	record := completedRecord()
	record.ConnectionLabel = "pve-roaminal"
	if _, err := service.SetPreference(context.Background(), PreferenceInput{ConnectionDefinitionID: "definition-1", TmuxSessionName: "team", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	service.Notify(record)
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	call := sender.lastCall()
	var payload map[string]any
	if err := json.Unmarshal(call.payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["messageId"] != "message-1" || payload["body"] != "pve-roaminal: Agent state: running -> relax" || payload["severity"] != "success" {
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
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", SessionActive: alwaysActive,
		Sender: sender, PreferenceRepository: repo, UserKey: "test-user", RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	input := validSubscriptionInput(t)
	if _, err := service.Register(context.Background(), "session-1", input); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetPreference(context.Background(), PreferenceInput{ConnectionDefinitionID: "definition-1", TmuxSessionName: "team", Enabled: true}); err != nil {
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
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", SessionActive: alwaysActive,
		Sender: sender2, PreferenceRepository: repo2, UserKey: "test-user", RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service2.Close()
	if _, err := service2.Register(context.Background(), "session-1", input); err != nil {
		t.Fatal(err)
	}
	if _, err := service2.SetPreference(context.Background(), PreferenceInput{ConnectionDefinitionID: "definition-1", TmuxSessionName: "team", Enabled: true}); err != nil {
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

func alwaysActive(string) (bool, error) { return true, nil }

func TestServicePreservesUnverifiedSubscriptionsAndBlocksDelivery(t *testing.T) {
	repo := &fakeRepository{}
	sender := &fakeSender{outcomes: []SendOutcome{{StatusCode: 503, Retryable: true}, {StatusCode: 201}}}
	checkErr := errors.New("enrollment read unavailable")
	active, authErr := false, checkErr
	checksBeforeFailure := -1
	service, err := New(repo, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com",
		Sender: sender, PreferenceRepository: repo, UserKey: "test-user", RetryDelay: time.Millisecond,
		SessionActive: func(string) (bool, error) {
			if checksBeforeFailure == 0 {
				return false, checkErr
			}
			checksBeforeFailure--
			return active, authErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if _, err := service.Register(t.Context(), "session-1", validSubscriptionInput(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetPreference(t.Context(), PreferenceInput{ConnectionDefinitionID: "definition-1", TmuxSessionName: "team", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.Prune(t.Context()); !errors.Is(err, checkErr) || len(repo.records) != 1 {
		t.Fatalf("unverified subscription pruned: %v", err)
	}
	notify := func() {
		t.Helper()
		service.Notify(completedRecord())
		if err := service.Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	notify()
	if sender.calls() != 0 || len(repo.records) != 1 {
		t.Fatal("unverified subscription was sent to or pruned")
	}
	active, authErr, checksBeforeFailure = true, nil, 1
	notify()
	if sender.calls() != 1 || len(repo.records) != 1 {
		t.Fatal("delivery did not resume or retry ignored the failed session check")
	}
	active, checksBeforeFailure = false, -1
	notify()
	if sender.calls() != 1 {
		t.Fatal("inactive subscription was sent to")
	}
	if err := service.Prune(t.Context()); err != nil || len(repo.records) != 0 {
		t.Fatalf("confirmed inactive subscription not pruned: %v", err)
	}
}

func TestServiceRejectsInvalidSubscription(t *testing.T) {
	service, err := New(&fakeRepository{}, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", SessionActive: alwaysActive, Sender: &fakeSender{},
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

func TestServiceSuppressesUnsupportedTransitionsAndPreferences(t *testing.T) {
	repo := &fakeRepository{}
	sender := &fakeSender{}
	service, err := New(repo, &fakeIDs{}, Options{
		PublicKey: "test-public", PrivateKey: "test-private", Subject: "mailto:test@example.com", SessionActive: alwaysActive,
		Sender: sender, PreferenceRepository: repo, UserKey: "test-user", RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if _, err := service.Register(context.Background(), "session-1", validSubscriptionInput(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetPreference(context.Background(), PreferenceInput{ConnectionDefinitionID: "definition-1", TmuxSessionName: "team", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	nonEligible := completedRecord()
	nonEligible.AgentStateFrom = "relax"
	nonEligible.AgentStateTo = "running"
	service.Notify(nonEligible)
	legacy := completedRecord()
	legacy.Kind = "codex_turn_completed"
	service.Notify(legacy)
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.calls() != 0 {
		t.Fatalf("unsupported transitions were delivered: %d", sender.calls())
	}
	if _, err := service.SetPreference(context.Background(), PreferenceInput{ConnectionDefinitionID: "definition-1", TmuxSessionName: "team", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	service.Notify(completedRecord())
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.calls() != 0 {
		t.Fatalf("disabled preference was delivered: %d", sender.calls())
	}
}

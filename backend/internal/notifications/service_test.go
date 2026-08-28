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
	record := completedRecord()
	record.ConnectionLabel = "pve-roaminal"
	service.Notify(record)
	if err := service.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	call := sender.lastCall()
	var payload map[string]any
	if err := json.Unmarshal(call.payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["messageId"] != "message-1" || payload["body"] != "pve-roaminal: Codex turn finished" || payload["severity"] != "success" {
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

package persistence

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

func TestPushSubscriptionsSurviveRestartAndRemainSessionOwned(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositories(store).PushSubscriptions
	record := validPushSubscriptionRecord(t, "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002")
	if got, err := repository.UpsertPushSubscription(context.Background(), record); err != nil || got.ID != record.ID {
		t.Fatalf("upsert: got=%+v err=%v", got, err)
	}
	if removed, err := repository.DeletePushSubscription(context.Background(), "00000000-0000-4000-8000-000000000003", record.ID); err != nil || removed {
		t.Fatalf("wrong owner delete: removed=%t err=%v", removed, err)
	}

	restarted, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := NewRepositories(restarted).PushSubscriptions.ListPushSubscriptions(context.Background())
	if err != nil || len(records) != 1 || records[0].ID != record.ID || records[0].AuthenticationSessionID != record.AuthenticationSessionID {
		t.Fatalf("after restart: records=%+v err=%v", records, err)
	}
	if removed, err := NewRepositories(restarted).PushSubscriptions.DeletePushSubscription(context.Background(), record.AuthenticationSessionID, record.ID); err != nil || !removed {
		t.Fatalf("owner delete: removed=%t err=%v", removed, err)
	}
}

func validPushSubscriptionRecord(t *testing.T, id, sessionID string) domain.PushSubscriptionRecord {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return domain.PushSubscriptionRecord{
		ID: id, AuthenticationSessionID: sessionID, Endpoint: "https://push.example.test/send/token",
		AuthKey: base64.RawURLEncoding.EncodeToString(auth), P256dhKey: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		CreatedAt: now, UpdatedAt: now,
	}
}

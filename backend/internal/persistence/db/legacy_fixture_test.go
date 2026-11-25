package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/agent"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/domain"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var legacyCheckpoint = []byte("imported terminal output 雪\r\n")

// Use the file-era writers for current formats, and literal envelopes for
// historical formats that those writers can no longer produce.
func writeLegacyFixture(t *testing.T, state, root string) {
	t.Helper()
	store, err := persistence.New(root)
	requireTransferTestSuccess(t, err)
	repos := persistence.NewRepositories(store)
	ctx := t.Context()
	login := sampleAuth()
	login.LastSeenAt = login.LastSeenAt.Add(123)
	stale, unbound := login, login
	stale.ID, stale.EnrollmentID = notificationTestID(90), notificationTestID(91)
	unbound.ID, unbound.EnrollmentID = notificationTestID(92), ""
	requireTransferTestSuccess(t, repos.TOTPEnrollment.SaveEnrollment(ctx, sampleEnrollment(81)))
	requireTransferTestSuccess(t, repos.Auth.SaveAuth(ctx, []domain.AuthSessionRecord{stale, login, unbound}))
	requireTransferTestSuccess(t, repos.Workspace.SaveWorkspaceLayout(ctx, domain.AuthenticationSessionID(login.ID), workspaceTestLayout(1), 0))

	now := time.Now().UTC().Truncate(time.Second).Add(123456789)
	for i := 1; i <= 3; i++ {
		_, _, err := repos.Messages.AppendMessage(messageDraft(i, now))
		requireTransferTestSuccess(t, err)
	}
	_, err = repos.Messages.MarkMessagesReadThrough(1)
	requireTransferTestSuccess(t, err)
	_, _, err = repos.Messages.DeleteMessage(messageDraft(2, now).MessageID)
	requireTransferTestSuccess(t, err)
	for _, i := range []int{8, 1} {
		push := pushFixture(t, i)
		push.AuthenticationSessionID = login.ID
		_, err = repos.PushSubscriptions.UpsertPushSubscription(ctx, push)
		requireTransferTestSuccess(t, err)
		_, err = repos.NotificationPreferences.UpsertNotificationPreference(ctx, preferenceFixture(i))
		requireTransferTestSuccess(t, err)
	}

	endpoints := agent.OpenStore(root)
	key, endpoint := agentEndpointFixture(t)
	endpoint.Targets["work"] = domain.AgentTargetState{SessionName: "work", LastSequence: 8, LastTransitionKey: "hook\x009"}
	requireTransferTestSuccess(t, endpoints.Update(key, func(out *agent.EndpointRecord) error {
		*out = endpoint
		return nil
	}))
	// An older scheduler could persist a target before full endpoint discovery.
	unresolved, err := agent.NormalizeEndpoint(ports.EffectiveEndpoint{User: "alice", Host: "unavailable.example.test", Port: 22})
	requireTransferTestSuccess(t, err)
	requireTransferTestSuccess(t, endpoints.Update(unresolved.Key, func(out *agent.EndpointRecord) error {
		out.Targets["work"] = endpoint.Targets["work"]
		return nil
	}))

	options := connectionoptions.New(state)
	requireTransferTestSuccess(t, options.UpdateAlias("retired-alias", connectionoptions.Tmux{Enabled: true, SessionName: "work", Pwd: "/workspace/project"}, true))
	job := uploadTestRecord()
	job.Status, job.Failures = "running", nil
	requireTransferTestSuccess(t, store.SaveUpload(ctx, job))
	writeLegacyBytes(t, filepath.Join(state, "uploads", "upload-interrupted", "part-a"), []byte("not database state"))
	writeLegacyInstances(t, store, root)
}

func writeLegacyInstances(t *testing.T, store *persistence.Store, root string) {
	t.Helper()
	header := domain.SnapshotHeader{Cols: 80, Rows: 24, ScrollbackLines: 1000, ThroughSequence: "9007199254740993"}
	for _, id := range []domain.ConnectionInstanceID{firstInstance, secondInstance} {
		meta := instanceFixture(id)
		meta.AutomaticTitle = "Imported"
		if id == secondInstance {
			meta.Lifecycle = "exited"
		}
		requireTransferTestSuccess(t, store.SaveConnectionInstance(meta))
		requireTransferTestSuccess(t, store.SaveSnapshot(id.String(), header, legacyCheckpoint))
	}
	requireTransferTestSuccess(t, store.ArchiveConnectionInstance(secondInstance.String()))
	requireTransferTestSuccess(t, store.DeleteConnectionInstance(secondInstance.String()))

	const flatV1 = `{"formatVersion":1,"id":"30000000-0000-4000-8000-000000000000","title":"Version one","initialCwd":"/workspace","cwd":"/workspace","cols":80,"rows":24,"createdAt":"2024-04-01T12:00:00Z","updatedAt":"2024-04-01T12:00:01Z","executions":[]}`
	const flatV2 = `{"formatVersion":2,"id":"40000000-0000-4000-8000-000000000000","automaticTitle":"Automatic","titleOverride":"Version two","initialCwd":"/workspace","cwd":"/workspace/project","cols":80,"rows":24,"createdAt":"2024-04-01T12:00:00Z","updatedAt":"2024-04-01T12:00:01.123456789Z","executions":null}`
	writeLegacyBytes(t, filepath.Join(root, "sessions", thirdInstance.String()+".json"), []byte(flatV1))
	writeLegacyBytes(t, filepath.Join(root, "sessions", flatV2ID+".json"), []byte(flatV2))
	raw, err := persistence.EncodeSnapshot(header, legacyCheckpoint)
	requireTransferTestSuccess(t, err)
	writeLegacyBytes(t, filepath.Join(root, "sessions", flatV2ID+".snapshot"), raw)
}

const flatV2ID = "40000000-0000-4000-8000-000000000000"

func writeLegacyBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	requireTransferTestSuccess(t, os.MkdirAll(filepath.Dir(path), 0o700))
	requireTransferTestSuccess(t, os.WriteFile(path, data, 0o600))
}

func writeLegacyJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	requireTransferTestSuccess(t, err)
	writeLegacyBytes(t, path, data)
}

func readLegacyFixture(t *testing.T, state, root string) persistence.LegacyData {
	t.Helper()
	loc, err := persistence.FindLegacyLocation(state, "")
	requireTransferTestSuccess(t, err)
	if !loc.Present || loc.Root != root {
		t.Fatalf("unexpected legacy root: %+v", loc)
	}
	data, err := persistence.ReadLegacy(loc, "")
	requireTransferTestSuccess(t, err)
	return data
}

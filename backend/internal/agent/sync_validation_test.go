package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestValidateSnapshotRequiresDeclaredCapabilities(t *testing.T) {
	snapshot := syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1)
	snapshot.Capabilities.Running = false
	if err := validateSnapshot(snapshot, "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("unsupported running capability error = %v", err)
	}
	snapshot = syncSnapshot("team", "$1", 10, "0123456789abcdef", "error", 1)
	snapshot.Capabilities.Error = false
	if err := validateSnapshot(snapshot, "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("unsupported error capability error = %v", err)
	}
	snapshot = syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1)
	snapshot.Records[0].Source = "startup"
	snapshot.Records[0].Reason = "user_requested"
	snapshot.Records[0].ProviderSessionID = "provider-session"
	snapshot.Records[0].TurnID = "turn"
	snapshot.Records[0].ToolUseID = "tool"
	if err := validateSnapshot(snapshot, "team"); err != nil {
		t.Fatalf("safe provider metadata was rejected: %v", err)
	}
	snapshot.Records[0].ToolUseID = strings.Repeat("x", 129)
	if err := validateSnapshot(snapshot, "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("oversized provider metadata error = %v", err)
	}
}

func TestReadRemoteStateRejectsTrailingJSON(t *testing.T) {
	view := syncView("instance", "definition", "team")
	output := append(
		encodeSyncSnapshot(t, syncSnapshot("team", "$1", 10, "0123456789abcdef", "running", 1)),
		[]byte(`{"extra":true}`)...,
	)
	terms := &syncConnectionService{
		views: []ports.ConnectionInstanceView{view},
		remote: map[string]syncRemoteResult{
			"instance": {result: ports.RemoteResult{Output: output}},
		},
	}
	service := newSyncService(t, terms, nil)
	if _, err := service.readRemoteState(context.Background(), "instance", "team"); !errors.Is(err, errRemoteStateInvalid) {
		t.Fatalf("trailing JSON error = %v, want invalid state", err)
	}
}

func TestSyncClassifiesMissingTmuxSessionSeparately(t *testing.T) {
	view := syncView("instance", "definition", "team")
	terms := &syncConnectionService{
		views:     []ports.ConnectionInstanceView{view},
		endpoints: map[string]ports.EffectiveEndpoint{"instance": {User: "coder", Host: "host.test", Port: 22}},
		remote: map[string]syncRemoteResult{
			"instance": {result: ports.RemoteResult{ErrorOutput: []byte(`{"error":"tmux session unavailable","code":"tmux_session_missing"}`)}, err: errors.New("remote command failed")},
		},
	}
	service := newSyncService(t, terms, nil)
	service.SyncOnce(context.Background())
	endpoint, err := NormalizeEndpoint(terms.endpoints["instance"])
	if err != nil {
		t.Fatal(err)
	}
	record, ok := service.store.Get(endpoint.Key)
	if !ok || record.Targets["team"].SyncStatus != syncStatusTmuxMissing {
		t.Fatalf("missing tmux session was not classified separately: ok=%v record=%+v", ok, record)
	}
}

func TestSafeSyncErrorIsBounded(t *testing.T) {
	message := safeSyncError(errors.New(fmt.Sprintf("%01000d", 1)))
	if len(message) != 256 {
		t.Fatalf("safe sync error length = %d, want 256", len(message))
	}
}

package report

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

func TestEventActivityMapping(t *testing.T) {
	if Activity("PermissionRequest", "") != "waiting" || Activity("Stop", "") != "completed" || Activity("SessionStart", "compact") != "running" || Activity("SessionStart", "startup") != "idle" {
		t.Fatal("unexpected activity mapping")
	}
}

func TestEventIDChangesWithSequence(t *testing.T) {
	base := NewEvent(map[string]string{"hook_event_name": "PreToolUse", "session_id": "session"}, tmux.Info{SessionID: "$0", SessionCreated: 1}, "endpoint", "1", 1)
	next := base
	next.Sequence = 2
	if EventID(base) == EventID(next) {
		t.Fatal("expected sequence to affect event ID")
	}
	if base.EventID == "" || model.SchemaVersion != 1 {
		t.Fatal("invalid event")
	}
}

func TestNewEventIncludesOpaqueAgentProcessID(t *testing.T) {
	event := NewEvent(map[string]string{"hook_event_name": "PreToolUse", "session_id": "session"}, tmux.Info{SessionID: "$0", SessionCreated: 1}, "endpoint", "1", 1)
	if event.Codex.AgentProcessID == "" || len(event.Codex.AgentProcessID) != 22 {
		t.Fatalf("missing opaque process identity: %+v", event.Codex)
	}
}

func TestReadInputKeepsOnlyEventMetadata(t *testing.T) {
	input, err := ReadInput(strings.NewReader(`{"hook_event_name":"PreToolUse","session_id":"s","prompt":"secret","cwd":"/private","source":"startup","reason":"secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if input["session_id"] != "s" || input["prompt"] != "" || input["cwd"] != "" || input["source"] != "" || input["reason"] != "" {
		t.Fatalf("unexpected metadata: %#v", input)
	}
	if KnownEvent("FutureEvent") || !KnownEvent("Stop") {
		t.Fatal("unexpected event allowlist result")
	}
}

func TestDrainRejectsBadEventWithoutRetainingPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	home := t.TempDir()
	info := tmux.Info{SessionID: "$0", SessionCreated: 1, SocketFingerprint: "0123456789abcdef"}
	event := model.Event{SchemaVersion: model.SchemaVersion, AgentType: "codex", EventID: "event-1", EventName: "PreToolUse", Activity: "running", Sequence: 1, Tmux: model.Tmux{SessionID: info.SessionID, SessionCreated: info.SessionCreated, SocketFingerprint: info.SocketFingerprint}}
	if err := WriteSpool(home, event, info); err != nil {
		t.Fatal(err)
	}
	Drain(context.Background(), model.Config{WebhookURL: server.URL, Token: "token"}, home, info)

	entries, err := os.ReadDir(SpoolPath(home, info))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			t.Fatalf("rejected event remained in spool: %s", entry.Name())
		}
	}
	rejected, err := os.ReadFile(filepath.Join(SpoolPath(home, info), "00000000000000000001-event-1.json.rejected"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := json.Unmarshal(rejected, &value); err != nil || value["code"] != "agent_event_invalid" || strings.Contains(string(rejected), "PreToolUse") {
		t.Fatalf("unexpected rejected record: %s", rejected)
	}
}

func TestWriteSpoolRepairsPrivateDirectoryPermissions(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".roaminal")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	info := tmux.Info{SessionID: "$0", SessionCreated: 1, SocketFingerprint: "0123456789abcdef"}
	event := model.Event{
		SchemaVersion: model.SchemaVersion,
		AgentType:     "codex",
		EventID:       "event-permissions",
		EventName:     "PreToolUse",
		Activity:      "running",
		Sequence:      1,
		Tmux:          model.Tmux{SessionID: info.SessionID, SessionCreated: info.SessionCreated, SocketFingerprint: info.SocketFingerprint},
	}
	if err := WriteSpool(home, event, info); err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if rootInfo.Mode().Perm() != 0700 {
		t.Fatalf("got root mode %o, want 0700", rootInfo.Mode().Perm())
	}
}

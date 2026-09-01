package terminal

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTerminatedMessageAllowsMissingExitStatus(t *testing.T) {
	var message StatusMessage
	if err := json.Unmarshal(terminatedStreamMessage(nil)(StreamEnvelope{SchemaVersion: ProtocolSchemaVersion, Sequence: 1, EventID: "event", OccurredAt: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}), &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "status" || message.Status != "terminated" || message.ExitStatus != nil {
		t.Fatalf("message = %+v", message)
	}
}

func TestMetaMessageIncludesTerminalType(t *testing.T) {
	var message MetaMessage
	data := metaStreamMessage(MetaMessage{Title: "shell", TitleMode: "automatic", Cwd: "/workspace", Cols: 80, Rows: 24, TerminalType: "xterm-256color"})(StreamEnvelope{SchemaVersion: ProtocolSchemaVersion, Sequence: 1, EventID: "event", OccurredAt: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)})
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "meta" || message.TerminalType != "xterm-256color" {
		t.Fatalf("message = %+v", message)
	}
}

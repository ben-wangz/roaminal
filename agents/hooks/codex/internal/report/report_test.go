package report

import (
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
)

func TestStateForCodexCompatibilityMapping(t *testing.T) {
	for _, test := range []struct {
		event  string
		source string
		reason string
		want   string
	}{
		{event: "SessionStart", source: "startup", want: model.StateRelax},
		{event: "SessionStart", source: "compact", want: model.StateRelax},
		{event: "UserPromptSubmit", want: model.StateRunning},
		{event: "PermissionRequest", want: model.StateRunning},
		{event: "PreCompact", want: model.StateRunning},
		{event: "PostCompact", want: model.StateRunning},
		{event: "PostToolUse", want: model.StateRunning},
		{event: "Stop", want: model.StateRelax},
		{event: "SessionEnd", reason: "other", want: model.StateRelax},
	} {
		got, ok := StateFor(test.event, test.source, test.reason)
		if !ok || got != test.want {
			t.Fatalf("StateFor(%q, %q, %q) = %q, %t; want %q, true", test.event, test.source, test.reason, got, ok, test.want)
		}
	}
	if _, ok := StateFor("FutureEvent", "", ""); ok {
		t.Fatal("unknown provider event was accepted")
	}
}

func TestReadInputKeepsOnlySafeEventMetadata(t *testing.T) {
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

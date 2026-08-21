package tmux

import "testing"

func TestParseInfo(t *testing.T) {
	info, err := parseInfo([]string{"roaminal", "$0", "1786613448", "%0", "/tmp/tmux-1000/default"})
	if err != nil {
		t.Fatal(err)
	}
	if info.SessionName != "roaminal" || info.SessionID != "$0" || info.PaneID != "%0" || info.SessionCreated != 1786613448 || len(info.SocketFingerprint) != 16 {
		t.Fatalf("unexpected tmux info: %+v", info)
	}
}

func TestParseInfoRejectsUnsafeValues(t *testing.T) {
	for _, parts := range [][]string{
		{"roaminal\nother", "$0", "1", "%0", "/tmp/tmux"},
		{"roaminal", "$0", "1", "%0", ""},
		{"roaminal", "$0", "-1", "%0", "/tmp/tmux"},
	} {
		if _, err := parseInfo(parts); err == nil {
			t.Fatalf("expected invalid identity for %#v", parts)
		}
	}
}

func TestSessionTargetUsesExactSessionTarget(t *testing.T) {
	if got := sessionTarget(Info{SessionName: "roaminal-e2e"}); got != "=roaminal-e2e:" {
		t.Fatalf("unexpected tmux session target: %q", got)
	}
}

func TestAgentProcessIDIsOpaqueAndStable(t *testing.T) {
	first := AgentProcessID("codex-session")
	second := AgentProcessID("codex-session")
	if first == "" || first != second || len(first) != 22 {
		t.Fatalf("unexpected process identity: %q %q", first, second)
	}
	if AgentProcessID("other-session") == first {
		t.Fatal("different Codex sessions must not share process identity")
	}
}

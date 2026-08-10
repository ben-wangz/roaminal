package connection

import (
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
)

func TestTmuxRemoteCommandPreflightsAndAttaches(t *testing.T) {
	command := tmuxRemoteCommand("Prod_1", "marker123")
	for _, expected := range []string{"command -v tmux", "tmux ls", "tmux-ready:marker123", "tmux new-session -A -s"} {
		if !strings.Contains(command, expected) {
			t.Fatalf("tmux command missing %q: %s", expected, command)
		}
	}
	if strings.Contains(command, "fallback") {
		t.Fatal("tmux command must not include a normal-shell fallback")
	}
}

func TestTmuxLaunchRevisionChangesWithSession(t *testing.T) {
	first := tmuxLaunchRevision(connectionoptions.Tmux{Enabled: true, SessionName: "t"})
	second := tmuxLaunchRevision(connectionoptions.Tmux{Enabled: true, SessionName: "u"})
	if first == second || len(first) != 64 {
		t.Fatalf("unexpected launch revisions: %q %q", first, second)
	}
}

func TestNormalizeTmuxPrefix(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "C-k\n", want: "k", ok: true},
		{input: "c-B", want: "b", ok: true},
		{input: "C-k extra", ok: false},
		{input: "C-1", ok: false},
		{input: "C-k\nC-j", ok: false},
	} {
		got, ok := normalizeTmuxPrefix(test.input)
		if got != test.want || ok != test.ok {
			t.Fatalf("normalizeTmuxPrefix(%q) = %q, %v; want %q, %v", test.input, got, ok, test.want, test.ok)
		}
	}
}

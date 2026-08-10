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

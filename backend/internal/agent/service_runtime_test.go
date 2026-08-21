package agent

import (
	"strings"
	"testing"
)

func TestTargetPreflightUsesExactTmuxSessionTarget(t *testing.T) {
	if !strings.Contains(tmuxTargetPreflightScript, `-t "=$1:"`) || !strings.Contains(tmuxTargetPreflightScript, "#{session_name}|#{session_id}|#{session_created}") {
		t.Fatalf("target preflight must address an exact tmux session: %s", tmuxTargetPreflightScript)
	}
	if strings.Contains(tmuxTargetPreflightScript, `-t "=$1" '#{session_name}`) {
		t.Fatalf("target preflight must include the session target separator: %s", tmuxTargetPreflightScript)
	}
}

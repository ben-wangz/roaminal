package filesystem

import (
	"strings"
	"testing"
)

func TestTmuxRootScriptUsesExactSessionTarget(t *testing.T) {
	if !strings.Contains(tmuxRootScript, `-t "=$session_name:"`) {
		t.Fatalf("filesystem root probe must address an exact tmux session: %s", tmuxRootScript)
	}
	if strings.Contains(tmuxRootScript, "-t \"=$session_name\"\n") || strings.Contains(tmuxRootScript, `-t "=$session_name" '#{`) {
		t.Fatalf("filesystem root probe must include the session target separator: %s", tmuxRootScript)
	}
}

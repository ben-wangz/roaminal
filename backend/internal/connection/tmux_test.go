package connection

import (
	"context"
	"errors"
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

func TestParseTmuxConfigPrefix(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		want      string
		found     bool
		supported bool
	}{
		{name: "set", input: "unbind C-b\nset -g prefix C-k\nbind C-k send-prefix\n", want: "k", found: true, supported: true},
		{name: "set option quoted", input: `set-option -g prefix "C-b"`, want: "b", found: true, supported: true},
		{name: "last setting wins", input: "set -g prefix C-a\nset -g prefix C-k\n", want: "k", found: true, supported: true},
		{name: "unrelated option named prefix", input: "set -g @other prefix C-k\n", found: false},
		{name: "comments and unrelated options", input: "# set -g prefix C-k\nset -g status on\n", found: false},
		{name: "unsupported value", input: "set -g prefix C-Space\n", found: true, supported: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, found, supported := parseTmuxConfigPrefix(test.input)
			if got != test.want || found != test.found || supported != test.supported {
				t.Fatalf("parseTmuxConfigPrefix(%q) = %q, %v, %v; want %q, %v, %v", test.input, got, found, supported, test.want, test.found, test.supported)
			}
		})
	}
}

func TestProbeTmuxPrefixUsesConfigAndDefaultsToCtrlB(t *testing.T) {
	transport := &Transport{Alias: "fixture"}
	var calls [][]string
	run := func(_ context.Context, _ *Transport, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[0] == "tmux" {
			return nil, errors.New("tmux server is not ready")
		}
		return []byte("unbind C-b\nset -g prefix C-k\n"), nil
	}
	key, source := probeTmuxPrefixWithRunner(context.Background(), transport, run)
	if key != "k" || source != "runtime" {
		t.Fatalf("probeTmuxPrefix with config = %q, %q; want k, runtime", key, source)
	}
	if len(calls) < 2 || calls[len(calls)-1][0] != "sh" {
		t.Fatalf("probe did not inspect ~/.tmux.conf: %#v", calls)
	}
	if len(calls[len(calls)-1]) != 3 || calls[len(calls)-1][1] != "-c" || calls[len(calls)-1][2] != shellQuote(tmuxConfigProbeCommand) {
		t.Fatalf("config probe script was not shell-quoted: %#v", calls[len(calls)-1])
	}

	key, source = probeTmuxPrefixWithRunner(context.Background(), transport, func(_ context.Context, _ *Transport, args ...string) ([]byte, error) {
		if args[0] == "tmux" {
			return nil, errors.New("tmux unavailable")
		}
		return nil, errors.New("config missing")
	})
	if key != "b" || source != "fallback" {
		t.Fatalf("probe default = %q, %q; want b, fallback", key, source)
	}
}

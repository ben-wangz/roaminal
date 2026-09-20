package connection

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
)

func TestTmuxRemoteCommandPreflightsAndAttaches(t *testing.T) {
	command := tmuxRemoteCommand("Prod_1", "~/workspace", "marker123")
	for _, expected := range []string{"command -v tmux", "tmux has-session -t", "tmux-ready:marker123", "tmux new-session -A -s", "-c \"$start_dir\""} {
		if !strings.Contains(command, expected) {
			t.Fatalf("tmux command missing %q: %s", expected, command)
		}
	}
	if strings.Contains(command, "fallback") {
		t.Fatal("tmux command must not include a normal-shell fallback")
	}
}

func TestTmuxRemoteScriptUsesPwdOnlyWhenSessionIsMissing(t *testing.T) {
	script := tmuxRemoteScript("Prod_1", "/srv/project", "marker123")
	for _, expected := range []string{
		"configured_pwd=" + shellQuote("/srv/project"),
		"if [ \"$session_status\" -ne 0 ]",
		"if ! (cd \"$start_dir\"",
		"exec tmux new-session -A -s " + shellQuote("Prod_1") + " -c \"$start_dir\"",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("tmux script missing %q: %s", expected, script)
		}
	}
}

func TestTmuxRemoteScriptQuotesConfiguredPwd(t *testing.T) {
	pwd := "/srv/a path/'quoted'"
	script := tmuxRemoteScript("Prod_1", pwd, "marker123")
	if !strings.Contains(script, "configured_pwd="+shellQuote(pwd)) {
		t.Fatalf("configured pwd was not shell-quoted: %s", script)
	}
	if strings.Contains(script, "tmux new-session -A -s "+shellQuote("Prod_1")+" -c "+pwd) {
		t.Fatal("configured pwd was interpolated into the tmux command")
	}
}

func TestTmuxRemoteScriptStartsMissingSessionInConfiguredPwd(t *testing.T) {
	home := t.TempDir()
	configured := filepath.Join(home, "workspace")
	if err := os.Mkdir(configured, 0o755); err != nil {
		t.Fatal(err)
	}
	output, calls, err := runTmuxRemoteScriptWithFake(t, tmuxRemoteScript("Prod_1", "$HOME/workspace", "marker123"), home, 1)
	if err != nil {
		t.Fatalf("remote script failed: %v; output=%q calls=%q", err, output, calls)
	}
	if !strings.Contains(output, "tmux-ready:marker123") {
		t.Fatalf("remote script did not publish the ready marker: %q", output)
	}
	if !strings.Contains(calls, "has-session -t Prod_1\nnew-session -A -s Prod_1 -c "+configured) {
		t.Fatalf("unexpected tmux calls: %q", calls)
	}
}

func TestTmuxRemoteScriptDoesNotRequirePwdForExistingSession(t *testing.T) {
	home := t.TempDir()
	missing := filepath.Join(home, "deleted")
	output, calls, err := runTmuxRemoteScriptWithFake(t, tmuxRemoteScript("Prod_1", "$HOME/deleted", "marker123"), home, 0)
	if err != nil {
		t.Fatalf("existing-session attach failed: %v; output=%q calls=%q", err, output, calls)
	}
	if !strings.Contains(output, "tmux-ready:marker123") {
		t.Fatalf("existing-session attach did not publish the ready marker: %q", output)
	}
	if !strings.Contains(calls, "has-session -t Prod_1\nnew-session -A -s Prod_1 -c "+missing) {
		t.Fatalf("unexpected tmux calls: %q", calls)
	}
}

func TestTmuxRemoteScriptReportsUnavailablePwdBeforeMarker(t *testing.T) {
	home := t.TempDir()
	output, calls, err := runTmuxRemoteScriptWithFake(t, tmuxRemoteScript("Prod_1", "$HOME/missing", "marker123"), home, 1)
	if err == nil {
		t.Fatalf("expected missing start directory to fail; output=%q calls=%q", output, calls)
	}
	if !strings.Contains(output, "Roaminal: tmux start directory is not accessible") {
		t.Fatalf("missing clear start-directory error: %q", output)
	}
	if strings.Contains(output, "tmux-ready:marker123") || strings.Contains(calls, "new-session") {
		t.Fatalf("missing start directory published or launched tmux: output=%q calls=%q", output, calls)
	}
}

func runTmuxRemoteScriptWithFake(t *testing.T, script, home string, sessionStatus int) (string, string, error) {
	t.Helper()
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "tmux-calls")
	fakeTmux := []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> \"$TMUX_CALLS\"\nif [ \"$1\" = has-session ]; then exit %d; fi\nif [ \"$1\" = new-session ]; then exit 0; fi\nexit 2\n", sessionStatus))
	if err := os.WriteFile(filepath.Join(bin, "tmux"), fakeTmux, 0o755); err != nil {
		t.Fatal(err)
	}
	env := os.Environ()
	pathValue := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	pathSet := false
	for index, value := range env {
		if strings.HasPrefix(value, "PATH=") {
			env[index] = "PATH=" + pathValue
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "PATH="+pathValue)
	}
	env = append(env, "HOME="+home, "TMUX_CALLS="+calls)
	command := exec.Command("sh", "-c", script)
	command.Env = env
	output, err := command.CombinedOutput()
	callData, readErr := os.ReadFile(calls)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatal(readErr)
	}
	return string(output), string(callData), err
}

func TestTmuxLaunchRevisionChangesWithSession(t *testing.T) {
	first := tmuxLaunchRevision(connectionoptions.Tmux{Enabled: true, SessionName: "t"})
	second := tmuxLaunchRevision(connectionoptions.Tmux{Enabled: true, SessionName: "u"})
	if first == second || len(first) != 64 {
		t.Fatalf("unexpected launch revisions: %q %q", first, second)
	}
}

func TestTmuxLaunchRevisionChangesWithPwd(t *testing.T) {
	first := tmuxLaunchRevision(connectionoptions.Tmux{Enabled: true, SessionName: "t", Pwd: "$HOME/one"})
	second := tmuxLaunchRevision(connectionoptions.Tmux{Enabled: true, SessionName: "t", Pwd: "$HOME/two"})
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

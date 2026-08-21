package connection

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestRemoteCommandInvocationPreservesPayloadScriptArguments(t *testing.T) {
	command := RemoteCommand{Script: `printf '%s|%s' "$1" "$2"`, Args: []string{"a path", "$HOME"}, Stdin: strings.NewReader("payload")}
	invocation := strings.Join(remoteCommandInvocation(command), " ")
	process := exec.CommandContext(context.Background(), "sh", "-c", invocation)
	output, err := process.Output()
	if err != nil {
		t.Fatalf("run quoted invocation: %v", err)
	}
	if string(output) != "a path|$HOME" {
		t.Fatalf("quoted arguments changed: %q", output)
	}
}

func TestRemoteCommandInvocationQuotesScriptAndArguments(t *testing.T) {
	invocation := remoteCommandInvocation(RemoteCommand{Script: "printf ok", Args: []string{"a'b"}, Stdin: strings.NewReader("payload")})
	if len(invocation) != 5 || invocation[2] != "'printf ok'" || invocation[4] != "'a'\"'\"'b'" {
		t.Fatalf("unexpected remote invocation: %#v", invocation)
	}
}

package agent

import (
	"errors"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestParseRemotePlatformUsesTypedFields(t *testing.T) {
	result := parseRemotePlatform([]byte("os=linux\narch=x86_64\ntmux=1\ncodex=1\nignored=value\n"))
	if result.OS != "linux" || result.Arch != "x86_64" || !result.Tmux || !result.Codex {
		t.Fatalf("parsed platform = %+v", result)
	}
}

func TestHelperInstallErrorMapsOnlyKnownCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{name: "hooks", code: "hooks file permissions are unsafe", want: "agent_hooks_invalid"},
	}
	if err := helperInstallError([]byte(`{"code":"private_directory_unsafe"}`)); err == nil || err.(*Error).Code != "agent_install_failed" {
		t.Fatalf("private directory error = %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := helperInstallError([]byte(`{"error":"` + test.code + `"}`))
			if err == nil || err.(*Error).Code != test.want {
				t.Fatalf("got %v, want %s", err, test.want)
			}
		})
	}
	if helperInstallError([]byte(`{"error":"/remote/private/path"}`)) != nil {
		t.Fatal("unknown helper output must remain generic")
	}
}

func TestHelperInstallDiagnosticUsesStableCodes(t *testing.T) {
	if got := helperInstallDiagnostic([]byte(`{"error":"private directory permissions are unsafe","code":"private_directory_unsafe"}`)); got != "private_directory_unsafe" {
		t.Fatalf("diagnostic code = %q", got)
	}
	if got := helperInstallDiagnostic([]byte(`{"error":"private directory permissions are unsafe"}`)); got != "helper_install_failed" {
		t.Fatalf("legacy diagnostic code = %q", got)
	}
	if got := helperInstallDiagnostic([]byte(`{"error":"anything","code":"/secret"}`)); got != "helper_install_failed" {
		t.Fatalf("unknown diagnostic code = %q", got)
	}
	if got := helperInstallDiagnostic([]byte("not-json")); got != "remote_output_unstructured" {
		t.Fatalf("unstructured diagnostic code = %q", got)
	}
}

func TestRemoteAgentErrorPreservesTransportFailure(t *testing.T) {
	err := remoteAgentError("agent_install_failed", 502, "install failed", ports.ErrTransportUnavailable)
	var agentErr *Error
	if !errors.As(err, &agentErr) || agentErr.Code != "agent_transport_unavailable" || agentErr.Status != 503 {
		t.Fatalf("mapped transport error = %v, want agent_transport_unavailable/503", err)
	}
	if !errors.Is(err, ports.ErrTransportUnavailable) {
		t.Fatalf("mapped transport error lost cause: %v", err)
	}
	ordinary := remoteAgentError("agent_install_failed", 502, "install failed", errors.New("remote helper failed"))
	if !errors.As(ordinary, &agentErr) || agentErr.Code != "agent_install_failed" || agentErr.Status != 502 {
		t.Fatalf("ordinary error was remapped: %v", ordinary)
	}
}

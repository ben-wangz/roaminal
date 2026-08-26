package agent

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func TestWebhookURLUsesV2AgentEventRoute(t *testing.T) {
	service := &Service{cfg: config.Config{AgentWebhookBaseURL: "https://roaminal.test/base"}}
	webhookURL, origin, err := service.webhookURL("")
	if err != nil {
		t.Fatalf("webhook URL: %v", err)
	}
	if origin != "https://roaminal.test/base" {
		t.Fatalf("origin = %q", origin)
	}
	if webhookURL != "https://roaminal.test/base/api/v2/agent/events" {
		t.Fatalf("webhook URL = %q", webhookURL)
	}
}

func TestParseRemotePlatformUsesTypedFields(t *testing.T) {
	result := parseRemotePlatform([]byte("os=linux\narch=x86_64\ntmux=1\ncodex=1\nignored=value\n"))
	if result.OS != "linux" || result.Arch != "x86_64" || !result.Tmux || !result.Codex {
		t.Fatalf("parsed platform = %+v", result)
	}
}

func TestInstallRequestIncludesEndpointKey(t *testing.T) {
	data, err := json.Marshal(installRequest{
		SchemaVersion: 1,
		Endpoint:      installEndpoint{Key: "endpoint-key", User: "root", Host: "example.test", Port: 22},
		WebhookURL:    "https://example.test/api/v2/agent/events",
	})
	if err != nil {
		t.Fatalf("marshal install request: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode install request: %v", err)
	}
	endpoint, ok := value["endpoint"].(map[string]any)
	if !ok || endpoint["key"] != "endpoint-key" {
		t.Fatalf("install request omitted endpoint key: %s", data)
	}
}

func TestHelperInstallErrorMapsOnlyKnownCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{name: "endpoint", code: "endpoint_conflict", want: "agent_endpoint_conflict"},
		{name: "binding", code: "binding_changed", want: "agent_binding_conflict"},
		{name: "hooks", code: "hooks file permissions are unsafe", want: "agent_hooks_invalid"},
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

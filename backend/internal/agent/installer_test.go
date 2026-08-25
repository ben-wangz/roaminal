package agent

import (
	"encoding/json"
	"testing"

	"github.com/ben-wangz/roaminal/backend/internal/config"
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

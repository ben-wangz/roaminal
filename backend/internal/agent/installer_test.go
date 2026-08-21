package agent

import (
	"encoding/json"
	"testing"
)

func TestInstallRequestIncludesEndpointKey(t *testing.T) {
	data, err := json.Marshal(installRequest{
		SchemaVersion: 1,
		Endpoint:      installEndpoint{Key: "endpoint-key", User: "root", Host: "example.test", Port: 22},
		WebhookURL:    "https://example.test/api/agent/events",
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

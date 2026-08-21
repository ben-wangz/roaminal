package agent

type installEndpoint struct {
	Key  string `json:"key"`
	User string `json:"user,omitempty"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}

type installRequest struct {
	SchemaVersion                   int             `json:"schemaVersion"`
	Endpoint                        installEndpoint `json:"endpoint"`
	WebhookURL                      string          `json:"webhookUrl"`
	ExpectedCurrentTokenFingerprint string          `json:"expectedCurrentTokenFingerprint"`
	ReplacementToken                string          `json:"replacementToken,omitempty"`
	ComponentVersion                string          `json:"componentVersion"`
	ComponentSHA256                 string          `json:"componentSha256"`
	TmuxSessionName                 string          `json:"tmuxSessionName"`
}

func installEndpointFor(endpoint Endpoint) installEndpoint {
	return installEndpoint{Key: endpoint.Key, User: endpoint.User, Host: endpoint.Host, Port: endpoint.Port}
}

type remoteProbe struct {
	Configured       bool
	TokenFingerprint string `json:"tokenFingerprint"`
	EndpointKey      string `json:"endpointKey"`
	ComponentVersion string `json:"componentVersion"`
	ComponentSHA256  string `json:"componentSha256"`
	HooksConfigured  bool   `json:"hooksConfigured"`
}

package api

// Version identifies the public Roaminal 0.3 transport contract. HTTP, WebSocket,
// and terminal-worker adapters use the same major contract generation while
// keeping their wire-specific prefixes separate.
const (
	Version           = "roaminal.v2"
	HTTPPrefix        = "/api/v2"
	WebSocketPrefix   = "/ws/v2"
	WebSocketProtocol = Version
	WorkerProtocol    = "roaminal-terminal-worker/4"
)

// ErrorResponse is the stable public error envelope for all 0.3 HTTP routes.
// Details are intentionally opaque to this package; each feature supplies its
// own bounded, JSON-serializable conflict or capability details.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	Details   any    `json:"details,omitempty"`
}

type VersionResponse struct {
	Name                     string `json:"name"`
	Version                  string `json:"version"`
	APIVersion               string `json:"apiVersion"`
	BootID                   string `json:"bootId"`
	ClientDiagnosticsEnabled bool   `json:"clientDiagnosticsEnabled"`
}

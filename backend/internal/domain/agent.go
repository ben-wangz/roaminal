package domain

import "time"

// AgentEndpointRecord is the persisted endpoint binding and its per-tmux
// runtime projections. It belongs to the agent repository boundary rather
// than the HTTP representation.
type AgentEndpointRecord struct {
	User                   string                      `json:"user"`
	Host                   string                      `json:"host"`
	Port                   int                         `json:"port"`
	Aliases                []string                    `json:"aliases,omitempty"`
	ActiveTokenHash        string                      `json:"activeTokenHash"`
	PendingTokenHash       string                      `json:"pendingTokenHash,omitempty"`
	PendingCreatedAt       string                      `json:"pendingCreatedAt,omitempty"`
	PreviousTokenHash      string                      `json:"previousTokenHash,omitempty"`
	PreviousTokenExpiresAt string                      `json:"previousTokenExpiresAt,omitempty"`
	ComponentVersion       string                      `json:"componentVersion,omitempty"`
	ComponentSHA256        string                      `json:"componentSha256,omitempty"`
	WebhookOrigin          string                      `json:"webhookOrigin,omitempty"`
	InstallationState      string                      `json:"installationState"`
	CreatedAt              string                      `json:"createdAt"`
	UpdatedAt              string                      `json:"updatedAt"`
	Targets                map[string]AgentTargetState `json:"targets,omitempty"`
}

type AgentTargetState struct {
	SessionName      string    `json:"sessionName"`
	SessionID        string    `json:"sessionId,omitempty"`
	SessionCreated   int64     `json:"sessionCreated,omitempty"`
	Component        string    `json:"component"`
	ComponentVersion string    `json:"componentVersion,omitempty"`
	Activity         string    `json:"activity"`
	LastEventName    string    `json:"lastEventName,omitempty"`
	LastTurnID       string    `json:"lastTurnId,omitempty"`
	StoppedTurnID    string    `json:"stoppedTurnId,omitempty"`
	LastToolUseID    string    `json:"lastToolUseId,omitempty"`
	LastEventAt      time.Time `json:"lastEventAt,omitempty"`
	LastReceivedAt   time.Time `json:"lastReceivedAt,omitempty"`
	LastSequence     uint64    `json:"lastSequence,omitempty"`
	InitializationID string    `json:"initializationId,omitempty"`
	ErrorCode        string    `json:"errorCode,omitempty"`
	ErrorMessage     string    `json:"errorMessage,omitempty"`
}

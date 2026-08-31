package domain

import "time"

// AgentEndpointRecord is the persisted endpoint binding and its per-tmux
// runtime projections. It belongs to the agent repository boundary rather
// than the HTTP representation.
type AgentEndpointRecord struct {
	User                   string   `json:"user"`
	Host                   string   `json:"host"`
	Port                   int      `json:"port"`
	Aliases                []string `json:"aliases,omitempty"`
	ActiveTokenHash        string   `json:"activeTokenHash,omitempty"`
	PendingTokenHash       string   `json:"pendingTokenHash,omitempty"`
	PendingCreatedAt       string   `json:"pendingCreatedAt,omitempty"`
	PreviousTokenHash      string   `json:"previousTokenHash,omitempty"`
	PreviousTokenExpiresAt string   `json:"previousTokenExpiresAt,omitempty"`
	// Legacy webhook fields remain readable during the 0.3 migration. They
	// are never used for authentication, delivery, or component configuration.
	WebhookOrigin     string                      `json:"webhookOrigin,omitempty"`
	ComponentVersion  string                      `json:"componentVersion,omitempty"`
	ComponentSHA256   string                      `json:"componentSha256,omitempty"`
	InstallationState string                      `json:"installationState"`
	CreatedAt         string                      `json:"createdAt"`
	UpdatedAt         string                      `json:"updatedAt"`
	Targets           map[string]AgentTargetState `json:"targets,omitempty"`
}

type AgentTargetState struct {
	SessionName      string    `json:"sessionName"`
	Provider         string    `json:"provider,omitempty"`
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
	// State fields are the provider-neutral projection populated by the local
	// state synchronizer. The legacy activity fields remain readable so an
	// existing repository can be migrated without losing diagnostics.
	RuntimeID         string    `json:"runtimeId,omitempty"`
	SocketFingerprint string    `json:"socketFingerprint,omitempty"`
	State             string    `json:"state,omitempty"`
	StateIndex        uint64    `json:"stateIndex,omitempty"`
	StateUpdatedAt    time.Time `json:"stateUpdatedAt,omitempty"`
	SyncStatus        string    `json:"syncStatus,omitempty"`
	LastSyncedAt      time.Time `json:"lastSyncedAt,omitempty"`
	SyncError         string    `json:"syncError,omitempty"`
	LastTransitionKey string    `json:"lastTransitionKey,omitempty"`
}

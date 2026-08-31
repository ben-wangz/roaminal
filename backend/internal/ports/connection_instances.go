package ports

import "time"

// AgentSummary is the application projection embedded in a connection
// instance resource. Agent state projection owns how this projection is derived;
// terminal runtime code must not own its product contract.
type AgentSummary struct {
	AgentType        string `json:"agentType"`
	Support          string `json:"support"`
	SupportReason    string `json:"supportReason"`
	Component        string `json:"component"`
	ComponentVersion string `json:"componentVersion"`
	Activity         string `json:"activity"`
	ActivityLabel    string `json:"activityLabel"`
	LastEventName    string `json:"lastEventName"`
	LastEventAt      string `json:"lastEventAt"`
	InitializationID string `json:"initializationId"`
	ErrorCode        string `json:"errorCode"`
	ErrorMessage     string `json:"errorMessage"`
	State            string `json:"state"`
	StateLabel       string `json:"stateLabel"`
	StateIndex       uint64 `json:"stateIndex"`
	StateUpdatedAt   string `json:"stateUpdatedAt"`
	SyncStatus       string `json:"syncStatus"`
	LastSyncedAt     string `json:"lastSyncedAt"`
	SyncError        string `json:"syncError"`
}

// RemoteCapability describes whether an interactive SSH connection instance
// currently has a reusable control transport for auxiliary features such as
// FileSystem and the remote monitor.
type RemoteCapability struct {
	Status    string `json:"status"`
	Retryable bool   `json:"retryable"`
	Reason    string `json:"reason,omitempty"`
}

// TerminalInstanceSummary is the terminal feature's runtime projection. It is
// deliberately separate from the connection-instance resource so terminal
// code cannot own connection-level projections such as agent state.
type TerminalInstanceSummary struct {
	ID                     string    `json:"-"`
	ConnectionInstanceID   string    `json:"connectionInstanceId"`
	ConnectionDefinitionID string    `json:"connectionDefinitionId"`
	Type                   string    `json:"type"`
	Purpose                string    `json:"purpose"`
	Lifecycle              string    `json:"lifecycle"`
	SourceState            string    `json:"sourceState"`
	SourceHostAlias        *string   `json:"sourceHostAlias,omitempty"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
	Title                  string    `json:"title"`
	TitleMode              string    `json:"titleMode"`
	Cwd                    string    `json:"cwd"`
	Cols                   int       `json:"cols"`
	Rows                   int       `json:"rows"`
	Attention              bool      `json:"attention"`
	GenerationStatus       string    `json:"generationStatus,omitempty"`
	GenerationError        string    `json:"generationError,omitempty"`
	TmuxEnabled            bool      `json:"tmuxEnabled,omitempty"`
	TmuxSessionName        string    `json:"tmuxSessionName,omitempty"`
	TmuxPrefixKey          string    `json:"tmuxPrefixKey,omitempty"`
	TmuxPrefixSource       string    `json:"tmuxPrefixSource,omitempty"`
}

// ConnectionInstanceSummary is the application-facing resource projection.
type ConnectionInstanceSummary struct {
	ID                     string           `json:"-"`
	ConnectionInstanceID   string           `json:"connectionInstanceId"`
	ConnectionDefinitionID string           `json:"connectionDefinitionId"`
	Type                   string           `json:"type"`
	Purpose                string           `json:"purpose"`
	Lifecycle              string           `json:"lifecycle"`
	SourceState            string           `json:"sourceState"`
	SourceHostAlias        *string          `json:"sourceHostAlias,omitempty"`
	CreatedAt              time.Time        `json:"createdAt"`
	UpdatedAt              time.Time        `json:"updatedAt"`
	Title                  string           `json:"title"`
	TitleMode              string           `json:"titleMode"`
	Cwd                    string           `json:"cwd"`
	Cols                   int              `json:"cols"`
	Rows                   int              `json:"rows"`
	Attention              bool             `json:"attention"`
	GenerationStatus       string           `json:"generationStatus,omitempty"`
	GenerationError        string           `json:"generationError,omitempty"`
	TmuxEnabled            bool             `json:"tmuxEnabled,omitempty"`
	TmuxSessionName        string           `json:"tmuxSessionName,omitempty"`
	TmuxPrefixKey          string           `json:"tmuxPrefixKey,omitempty"`
	TmuxPrefixSource       string           `json:"tmuxPrefixSource,omitempty"`
	Agent                  AgentSummary     `json:"agent"`
	RemoteCapability       RemoteCapability `json:"remoteCapability"`
}

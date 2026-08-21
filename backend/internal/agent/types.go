package agent

import "time"

type Endpoint struct {
	Key     string `json:"-"`
	User    string `json:"user,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	Display string `json:"display,omitempty"`
}

type Target struct {
	EndpointKey string
	SessionName string
}

type EndpointRecord struct {
	User                   string                 `json:"user"`
	Host                   string                 `json:"host"`
	Port                   int                    `json:"port"`
	Aliases                []string               `json:"aliases,omitempty"`
	ActiveTokenHash        string                 `json:"activeTokenHash"`
	PendingTokenHash       string                 `json:"pendingTokenHash,omitempty"`
	PendingCreatedAt       string                 `json:"pendingCreatedAt,omitempty"`
	PreviousTokenHash      string                 `json:"previousTokenHash,omitempty"`
	PreviousTokenExpiresAt string                 `json:"previousTokenExpiresAt,omitempty"`
	ComponentVersion       string                 `json:"componentVersion,omitempty"`
	ComponentSHA256        string                 `json:"componentSha256,omitempty"`
	WebhookOrigin          string                 `json:"webhookOrigin,omitempty"`
	InstallationState      string                 `json:"installationState"`
	CreatedAt              string                 `json:"createdAt"`
	UpdatedAt              string                 `json:"updatedAt"`
	Targets                map[string]TargetState `json:"targets,omitempty"`
}

type TargetState struct {
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

type Initialization struct {
	ID                   string     `json:"initializationId"`
	ConnectionInstanceID string     `json:"connectionInstanceId,omitempty"`
	Endpoint             Endpoint   `json:"endpoint,omitempty"`
	TmuxSessionName      string     `json:"tmuxSessionName,omitempty"`
	WebhookURL           string     `json:"webhookUrl,omitempty"`
	Status               string     `json:"status"`
	Result               string     `json:"result,omitempty"`
	Component            string     `json:"component,omitempty"`
	Changed              bool       `json:"changed,omitempty"`
	Joined               bool       `json:"joined,omitempty"`
	PriorComponent       string     `json:"-"`
	Error                *SafeError `json:"error,omitempty"`
	StartedAt            time.Time  `json:"startedAt"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
}

type SafeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

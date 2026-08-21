package model

import "time"

const SchemaVersion = 1

type Config struct {
	FormatVersion    int      `json:"formatVersion"`
	AgentType        string   `json:"agentType"`
	Endpoint         Endpoint `json:"endpoint"`
	WebhookURL       string   `json:"webhookUrl"`
	Token            string   `json:"token"`
	TokenFingerprint string   `json:"tokenFingerprint"`
	ComponentVersion string   `json:"componentVersion"`
	ComponentSHA256  string   `json:"componentSha256"`
	UpdatedAt        string   `json:"updatedAt"`
}

type InstallRequest struct {
	SchemaVersion                   int      `json:"schemaVersion"`
	Endpoint                        Endpoint `json:"endpoint"`
	WebhookURL                      string   `json:"webhookUrl"`
	ExpectedCurrentTokenFingerprint string   `json:"expectedCurrentTokenFingerprint"`
	ReplacementToken                string   `json:"replacementToken,omitempty"`
	ComponentVersion                string   `json:"componentVersion"`
	ComponentSHA256                 string   `json:"componentSha256"`
	TmuxSessionName                 string   `json:"tmuxSessionName"`
}

type Endpoint struct {
	Key  string `json:"key"`
	User string `json:"user"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type ProbeResponse struct {
	SchemaVersion    int    `json:"schemaVersion"`
	ComponentVersion string `json:"componentVersion"`
	ComponentSHA256  string `json:"componentSha256,omitempty"`
	EndpointKey      string `json:"endpointKey,omitempty"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	TmuxAvailable    bool   `json:"tmuxAvailable"`
	CodexConfig      bool   `json:"codexConfig"`
	HooksConfigured  bool   `json:"hooksConfigured"`
	TokenFingerprint string `json:"tokenFingerprint,omitempty"`
}

type Tmux struct {
	SessionName       string `json:"sessionName"`
	SessionID         string `json:"sessionId"`
	SessionCreated    int64  `json:"sessionCreated"`
	PaneID            string `json:"paneId"`
	SocketFingerprint string `json:"socketFingerprint"`
}

type Codex struct {
	SessionID      string `json:"sessionId,omitempty"`
	TurnID         string `json:"turnId,omitempty"`
	ToolUseID      string `json:"toolUseId,omitempty"`
	AgentProcessID string `json:"agentProcessId,omitempty"`
}

type EventSource struct {
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type Event struct {
	EndpointKey      string      `json:"-"`
	SchemaVersion    int         `json:"schemaVersion"`
	AgentType        string      `json:"agentType"`
	ComponentVersion string      `json:"componentVersion"`
	EventID          string      `json:"eventId"`
	EventName        string      `json:"eventName"`
	Activity         string      `json:"activity"`
	Sequence         uint64      `json:"sequence"`
	OccurredAt       time.Time   `json:"occurredAt"`
	Tmux             Tmux        `json:"tmux"`
	Codex            Codex       `json:"codex,omitempty"`
	Event            EventSource `json:"event,omitempty"`
}

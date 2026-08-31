package model

import "time"

const SchemaVersion = 1

const (
	ProviderCodex   = "codex"
	StateRunning    = "running"
	StateRelax      = "relax"
	StateError      = "error"
	MaxStateRecords = 128
)

// ComponentConfig is local installation metadata. It deliberately contains
// no Roaminal endpoint, connection instance, URL, or credential.
type ComponentConfig struct {
	FormatVersion    int       `json:"formatVersion"`
	Provider         string    `json:"provider"`
	ComponentVersion string    `json:"componentVersion"`
	ComponentSHA256  string    `json:"componentSha256"`
	InstalledAt      time.Time `json:"installedAt"`
}

type StateCapabilities struct {
	Running bool `json:"running"`
	Relax   bool `json:"relax"`
	Error   bool `json:"error"`
}

type StateRecord struct {
	Index             uint64    `json:"index"`
	Timestamp         time.Time `json:"timestamp"`
	State             string    `json:"state"`
	EventName         string    `json:"eventName,omitempty"`
	Source            string    `json:"source,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	ProviderSessionID string    `json:"providerSessionId,omitempty"`
	TurnID            string    `json:"turnId,omitempty"`
	ToolUseID         string    `json:"toolUseId,omitempty"`
}

type StateFile struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Provider         string            `json:"provider"`
	ComponentVersion string            `json:"componentVersion"`
	Capabilities     StateCapabilities `json:"capabilities"`
	Tmux             Tmux              `json:"tmux"`
	RuntimeID        string            `json:"runtimeId"`
	State            string            `json:"state"`
	LatestIndex      uint64            `json:"latestIndex"`
	Records          []StateRecord     `json:"records"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

// LocalInstallRequest is the only install contract sent by Roaminal. It has no
// connection or network identity because the installed component is shared by
// the user's local Codex configuration.
type LocalInstallRequest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	ComponentVersion string `json:"componentVersion"`
	ComponentSHA256  string `json:"componentSha256"`
}

type Tmux struct {
	SessionName       string `json:"sessionName"`
	SessionID         string `json:"sessionId"`
	SessionCreated    int64  `json:"sessionCreated"`
	PaneID            string `json:"paneId"`
	SocketFingerprint string `json:"socketFingerprint"`
}

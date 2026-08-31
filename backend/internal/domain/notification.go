package domain

import "time"

// PushSubscriptionRecord is private server-side delivery data. Its endpoint
// and encryption keys must never be returned in a normal API response or log.
type PushSubscriptionRecord struct {
	ID                      string    `json:"id"`
	AuthenticationSessionID string    `json:"authenticationSessionId"`
	Endpoint                string    `json:"endpoint"`
	AuthKey                 string    `json:"authKey"`
	P256dhKey               string    `json:"p256dhKey"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// NotificationPreference is the durable preference for one
// connection-definition/tmux-session pair. UserKey is an internal stable
// identity and is never returned by an HTTP handler.
type NotificationPreference struct {
	UserKey                string    `json:"userKey"`
	ConnectionDefinitionID string    `json:"connectionDefinitionId"`
	TmuxSessionName        string    `json:"tmuxSessionName"`
	Enabled                bool      `json:"enabled"`
	RunningToRelax         bool      `json:"runningToRelax"`
	RunningToError         bool      `json:"runningToError"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

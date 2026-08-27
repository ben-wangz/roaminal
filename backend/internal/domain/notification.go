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

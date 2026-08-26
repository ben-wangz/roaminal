package domain

import "time"

// MessageDraft is the internal write contract for a durable Agent message.
// Endpoint and event identifiers never cross the HTTP presentation boundary.
type MessageDraft struct {
	MessageID             string    `json:"messageId"`
	IdempotencyKey        string    `json:"idempotencyKey"`
	Kind                  string    `json:"kind"`
	Severity              string    `json:"severity"`
	AgentType             string    `json:"agentType"`
	PresentationKey       string    `json:"presentationKey"`
	OccurredAt            time.Time `json:"occurredAt"`
	ReceivedAt            time.Time `json:"receivedAt"`
	EndpointKey           string    `json:"endpointKey"`
	FallbackLabel         string    `json:"fallbackLabel"`
	TmuxSessionName       string    `json:"tmuxSessionName"`
	TmuxSessionID         string    `json:"tmuxSessionId"`
	TmuxSessionCreated    int64     `json:"tmuxSessionCreated"`
	ConnectionInstanceIDs []string  `json:"connectionInstanceIds,omitempty"`
}

// MessageRecord is the persisted form of a message with its monotonic
// sequence. The presentation text is deliberately represented by a key.
type MessageRecord struct {
	MessageDraft
	Sequence uint64 `json:"sequence"`
}

type MessagePage struct {
	Messages            []MessageRecord
	NextBefore          uint64
	HasMore             bool
	Revision            uint64
	LatestSequence      uint64
	ReadThroughSequence uint64
	UnreadCount         int
}

type MessageState struct {
	Revision            uint64
	LatestSequence      uint64
	ReadThroughSequence uint64
	UnreadCount         int
}

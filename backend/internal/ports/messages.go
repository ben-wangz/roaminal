package ports

import (
	"errors"

	"github.com/ben-wangz/roaminal/backend/internal/domain"
)

var ErrMessageCursorInvalid = errors.New("message cursor is invalid")

// MessageRepository is the durable message boundary. Implementations own
// serialization, retention, and synchronization of the global read cursor.
type MessageRepository interface {
	AppendMessage(domain.MessageDraft) (domain.MessageRecord, bool, error)
	ListMessages(limit int, before uint64) (domain.MessagePage, error)
	MarkMessagesReadThrough(uint64) (domain.MessageState, error)
	MessageState() (domain.MessageState, error)
	DeleteMessage(string) (domain.MessageState, bool, error)
	ClearMessages() (domain.MessageState, int, error)
}

// MessageAppender is the small dependency used by Agent state projection. Keeping
// it separate from the query methods prevents Agent code from depending on
// HTTP or browser presentation concerns.
type MessageAppender interface {
	AppendMessage(domain.MessageDraft) (domain.MessageRecord, bool, error)
}

// AgentProjectionUpdate runs against the current, write-locked endpoint. It
// changes the checkpoint and optionally returns one transition to commit with it.
// The callback must not access repositories or deliver notifications.
type AgentProjectionUpdate func(*domain.AgentEndpointRecord) (*domain.MessageDraft, error)

// AgentProjectionRepository atomically persists a checkpoint and its optional
// message. An empty record means no message; a duplicate returns the existing
// record. The result is available only after a successful commit.
type AgentProjectionRepository interface {
	UpdateProjection(key string, update AgentProjectionUpdate) (record domain.MessageRecord, duplicate bool, err error)
}

// AgentProjectionRecorder prepares the optional message and handles notification
// after the checkpoint commits. Only AgentProjectionRepository implementations
// can commit the checkpoint and message atomically.
type AgentProjectionRecorder interface {
	CommitAgentProjection(AgentRepository, string, AgentProjectionUpdate) error
}

// MessageNotifier receives only newly persisted records. Implementations must
// return quickly and perform best-effort delivery asynchronously.
type MessageNotifier interface {
	Notify(domain.MessageRecord)
}

package terminal

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

const ProtocolSchemaVersion = 2

type StreamEnvelope struct {
	SchemaVersion int       `json:"schemaVersion"`
	Sequence      uint64    `json:"sequence"`
	EventID       string    `json:"eventId"`
	OccurredAt    time.Time `json:"occurredAt"`
}

// The terminal wire model is deliberately composed of concrete messages. The
// server still sends JSON text, but feature code no longer assembles arbitrary
// maps for protocol events.
type SnapshotMessage struct {
	StreamEnvelope
	Type string `json:"type"`
	Data string `json:"data"`
}

type MetaMessage struct {
	StreamEnvelope
	Type             string `json:"type"`
	Title            string `json:"title"`
	TitleMode        string `json:"titleMode"`
	Cwd              string `json:"cwd"`
	Cols             int    `json:"cols"`
	Rows             int    `json:"rows"`
	TerminalType     string `json:"terminalType"`
	Attention        bool   `json:"attention,omitempty"`
	SourceState      string `json:"sourceState,omitempty"`
	GenerationStatus string `json:"generationStatus,omitempty"`
	GenerationError  string `json:"generationError,omitempty"`
}

type StatusMessage struct {
	StreamEnvelope
	Type       string      `json:"type"`
	Status     string      `json:"status"`
	Code       int         `json:"code,omitempty"`
	Signal     *int        `json:"signal,omitempty"`
	ExitStatus *ExitStatus `json:"exitStatus,omitempty"`
}

type OutputMessage struct {
	StreamEnvelope
	Type string `json:"type"`
	Data string `json:"data"`
}

type ExecutionMessage struct {
	StreamEnvelope
	Type        string           `json:"type"`
	Phase       string           `json:"phase"`
	ExecutionID string           `json:"executionId"`
	Command     string           `json:"command,omitempty"`
	StartedAt   time.Time        `json:"startedAt,omitempty"`
	Entry       *executionRecord `json:"entry,omitempty"`
}

type LaunchPublishedMessage struct {
	StreamEnvelope
	Type     string  `json:"type"`
	Instance Summary `json:"instance"`
}

type PongMessage struct {
	StreamEnvelope
	Type string `json:"type"`
}

func encodeMessage(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

type streamMessage func(StreamEnvelope) []byte

func streamEnvelope(sequence uint64, occurredAt time.Time, ids ports.IDGenerator) StreamEnvelope {
	eventID := ""
	if ids != nil {
		eventID, _ = ids.NewID()
	}
	if eventID == "" {
		eventID = fmt.Sprintf("event-%d", occurredAt.UnixNano())
	}
	return StreamEnvelope{SchemaVersion: ProtocolSchemaVersion, Sequence: sequence, EventID: eventID, OccurredAt: occurredAt.UTC()}
}

func snapshotStreamMessage(data string) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		return encodeMessage(SnapshotMessage{StreamEnvelope: envelope, Type: "snapshot", Data: data})
	}
}

func metaStreamMessage(meta MetaMessage) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		meta.StreamEnvelope = envelope
		meta.Type = "meta"
		return encodeMessage(meta)
	}
}

func readyStreamMessage() streamMessage {
	return func(envelope StreamEnvelope) []byte {
		return encodeMessage(StatusMessage{StreamEnvelope: envelope, Type: "status", Status: "ready"})
	}
}

func terminatedStreamMessage(status *ExitStatus) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		message := StatusMessage{StreamEnvelope: envelope, Type: "status", Status: "terminated", Code: statusCode(status), ExitStatus: status}
		if status != nil {
			message.Signal = status.Signal
		}
		return encodeMessage(message)
	}
}

func outputStreamMessage(data string) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		return encodeMessage(OutputMessage{StreamEnvelope: envelope, Type: "output", Data: data})
	}
}

func executionStartedStreamMessage(id, command string, startedAt time.Time) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		return encodeMessage(ExecutionMessage{StreamEnvelope: envelope, Type: "execution", Phase: "started", ExecutionID: id, Command: command, StartedAt: startedAt})
	}
}

func executionCompletedStreamMessage(id string, entry executionRecord) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		return encodeMessage(ExecutionMessage{StreamEnvelope: envelope, Type: "execution", Phase: "completed", ExecutionID: id, Entry: &entry})
	}
}

func launchPublishedStreamMessage(summary Summary) streamMessage {
	return func(envelope StreamEnvelope) []byte {
		return encodeMessage(LaunchPublishedMessage{StreamEnvelope: envelope, Type: "launch_published", Instance: summary})
	}
}

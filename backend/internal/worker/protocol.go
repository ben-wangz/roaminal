package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

type requestHeader struct {
	Op              string `json:"op"`
	Protocol        string `json:"protocol"`
	SchemaVersion   int    `json:"schemaVersion"`
	CorrelationID   string `json:"correlationId"`
	TerminalID      string `json:"terminalId,omitempty"`
	Cols            *int   `json:"cols,omitempty"`
	Rows            *int   `json:"rows,omitempty"`
	ScrollbackLines *int   `json:"scrollbackLines,omitempty"`
	Sequence        string `json:"sequence,omitempty"`
	ThroughSequence string `json:"throughSequence,omitempty"`
}

type responseHeader struct {
	Op                    string    `json:"op"`
	Protocol              string    `json:"protocol"`
	SchemaVersion         int       `json:"schemaVersion"`
	CorrelationID         string    `json:"correlationId"`
	Sequence              string    `json:"sequence"`
	EventID               string    `json:"eventId"`
	OccurredAt            time.Time `json:"occurredAt"`
	RequestOp             string    `json:"requestOp,omitempty"`
	TerminalID            string    `json:"terminalId,omitempty"`
	ThroughSequence       string    `json:"throughSequence,omitempty"`
	Engine                string    `json:"engine,omitempty"`
	EngineVersion         string    `json:"engineVersion,omitempty"`
	SerializeAddonVersion string    `json:"serializeAddonVersion,omitempty"`
	Code                  string    `json:"code,omitempty"`
	Message               string    `json:"message,omitempty"`
	Fatal                 bool      `json:"fatal,omitempty"`
}

func intValue(value int) *int { return &value }

func decodeResponseHeader(header json.RawMessage) (responseHeader, error) {
	decoder := json.NewDecoder(bytes.NewReader(header))
	decoder.DisallowUnknownFields()
	var result responseHeader
	if err := decoder.Decode(&result); err != nil {
		return responseHeader{}, errors.New("invalid worker response header")
	}
	if result.Op == "" || result.Protocol != Protocol || result.SchemaVersion != SchemaVersion || result.CorrelationID == "" || result.EventID == "" || result.OccurredAt.IsZero() || !validSequence(result.Sequence) || result.Sequence == "0" {
		return responseHeader{}, errors.New("invalid worker response contract")
	}
	return result, nil
}

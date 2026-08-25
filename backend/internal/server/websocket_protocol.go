package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type websocketCommand struct {
	Type      string
	RequestID string
	Data      string
	Cols      int
	Rows      int
}

type websocketCommandWire struct {
	Type      string  `json:"type"`
	RequestID *string `json:"requestId,omitempty"`
	Data      *string `json:"data,omitempty"`
	Cols      *int    `json:"cols,omitempty"`
	Rows      *int    `json:"rows,omitempty"`
}

type websocketPong struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
}

func decodeWebSocketCommand(data []byte) (websocketCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire websocketCommandWire
	if err := decoder.Decode(&wire); err != nil {
		return websocketCommand{}, err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return websocketCommand{Type: wire.Type}, errors.New("multiple websocket messages")
		}
		return websocketCommand{Type: wire.Type}, err
	}
	switch wire.Type {
	case "input":
		if wire.RequestID == nil || strings.TrimSpace(*wire.RequestID) == "" || len(*wire.RequestID) > 128 || wire.Data == nil || wire.Cols != nil || wire.Rows != nil {
			return websocketCommand{Type: wire.Type}, errors.New("invalid input command")
		}
		return websocketCommand{Type: wire.Type, RequestID: *wire.RequestID, Data: *wire.Data}, nil
	case "resize":
		if wire.RequestID == nil || strings.TrimSpace(*wire.RequestID) == "" || len(*wire.RequestID) > 128 || wire.Data != nil || wire.Cols == nil || wire.Rows == nil || *wire.Cols < 2 || *wire.Cols > 1000 || *wire.Rows < 1 || *wire.Rows > 1000 {
			return websocketCommand{Type: wire.Type}, errors.New("invalid resize command")
		}
		return websocketCommand{Type: wire.Type, RequestID: *wire.RequestID, Cols: *wire.Cols, Rows: *wire.Rows}, nil
	case "ping", "claim_terminal_control":
		if wire.RequestID == nil || strings.TrimSpace(*wire.RequestID) == "" || len(*wire.RequestID) > 128 || wire.Data != nil || wire.Cols != nil || wire.Rows != nil {
			return websocketCommand{Type: wire.Type}, errors.New("invalid websocket command")
		}
		return websocketCommand{Type: wire.Type, RequestID: *wire.RequestID}, nil
	default:
		return websocketCommand{Type: wire.Type}, errors.New("unknown websocket command")
	}
}

func isWebSocketControlCommand(commandType string) bool {
	switch commandType {
	case "input", "resize", "claim_terminal_control":
		return true
	default:
		return false
	}
}

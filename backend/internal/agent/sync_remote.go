package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

var (
	errRemoteStateMissing = errors.New("remote agent state is missing")
	errRemoteStateInvalid = errors.New("remote agent state is invalid")
	errRemoteTmuxMissing  = errors.New("configured tmux session is missing")
)

type remoteAgentState struct {
	SchemaVersion    int                 `json:"schemaVersion"`
	Provider         string              `json:"provider"`
	ComponentVersion string              `json:"componentVersion"`
	Capabilities     remoteCapabilities  `json:"capabilities"`
	Tmux             remoteTmuxIdentity  `json:"tmux"`
	RuntimeID        string              `json:"runtimeId"`
	State            string              `json:"state"`
	LatestIndex      uint64              `json:"latestIndex"`
	Records          []remoteStateRecord `json:"records"`
	UpdatedAt        time.Time           `json:"updatedAt"`
}

type remoteCapabilities struct {
	Running bool `json:"running"`
	Relax   bool `json:"relax"`
	Error   bool `json:"error"`
}

type remoteTmuxIdentity struct {
	SessionName       string `json:"sessionName"`
	SessionID         string `json:"sessionId"`
	SessionCreated    int64  `json:"sessionCreated"`
	PaneID            string `json:"paneId"`
	SocketFingerprint string `json:"socketFingerprint"`
}

type remoteStateRecord struct {
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

func (s *Service) readRemoteState(ctx context.Context, id, sessionName string) (remoteAgentState, error) {
	result, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{
		Script: "set -eu\n\"$HOME/.roaminal/bin/roaminal-agent-hook\" read-state --session \"$1\"\n",
		Args:   []string{sessionName}, Timeout: 10 * time.Second, OutputLimit: 512 * 1024,
	})
	if err != nil {
		return remoteAgentState{}, classifyRemoteStateError(result.ErrorOutput, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(result.Output))
	decoder.DisallowUnknownFields()
	var value remoteAgentState
	if err := decoder.Decode(&value); err != nil {
		return remoteAgentState{}, fmt.Errorf("%w: decode state: %v", errRemoteStateInvalid, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return remoteAgentState{}, fmt.Errorf("%w: multiple state values", errRemoteStateInvalid)
	}
	return value, nil
}

func classifyRemoteStateError(output []byte, cause error) error {
	if errors.Is(cause, ports.ErrTransportUnavailable) {
		return cause
	}
	var value struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(output, &value) == nil {
		switch value.Code {
		case "state_missing":
			return errRemoteStateMissing
		case "tmux_session_missing":
			return errRemoteTmuxMissing
		case "state_runtime_mismatch", "state_invalid":
			return errRemoteStateInvalid
		}
	}
	lower := strings.ToLower(string(output) + " " + cause.Error())
	if strings.Contains(lower, "tmux session") || strings.Contains(lower, "can't find session") {
		return errRemoteTmuxMissing
	}
	if strings.Contains(lower, "permission denied") || strings.Contains(lower, "unsafe") {
		return fmt.Errorf("%w: permission or path rejected", errRemoteStateInvalid)
	}
	return cause
}

func validateSnapshot(value remoteAgentState, sessionName string) error {
	if value.SchemaVersion != 1 || strings.TrimSpace(value.Provider) == "" || value.ComponentVersion == "" || !validAgentState(value.State) || value.LatestIndex == 0 || value.UpdatedAt.IsZero() {
		return errRemoteStateInvalid
	}
	if !value.Capabilities.Running || !value.Capabilities.Relax || (value.State == "error" && !value.Capabilities.Error) {
		return errRemoteStateInvalid
	}
	if !validRemoteMetadata(value.Provider) || !validRemoteMetadata(value.ComponentVersion) || value.Tmux.SessionName != sessionName || value.Tmux.SessionID == "" || !validRemoteMetadata(value.Tmux.SessionID) || value.Tmux.SessionCreated < 0 || value.Tmux.PaneID == "" || !validRemoteMetadata(value.Tmux.PaneID) || !validSocketFingerprint(value.Tmux.SocketFingerprint) {
		return errRemoteStateInvalid
	}
	if value.RuntimeID != runtimeID(value.Tmux) || len(value.Records) == 0 || len(value.Records) > 128 {
		return errRemoteStateInvalid
	}
	var previous uint64
	for _, record := range value.Records {
		if record.Index == 0 || record.Index <= previous || record.Index > value.LatestIndex || record.Timestamp.IsZero() || !validAgentState(record.State) || !validRemoteMetadata(record.EventName) || !validRemoteMetadata(record.Source) || !validRemoteMetadata(record.Reason) || !validRemoteMetadata(record.ProviderSessionID) || !validRemoteMetadata(record.TurnID) || !validRemoteMetadata(record.ToolUseID) {
			return errRemoteStateInvalid
		}
		previous = record.Index
	}
	last := value.Records[len(value.Records)-1]
	if last.Index != value.LatestIndex || last.State != value.State {
		return errRemoteStateInvalid
	}
	return nil
}

func validAgentState(value string) bool {
	return value == "running" || value == "relax" || value == "error"
}

func validRemoteMetadata(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func runtimeID(value remoteTmuxIdentity) string {
	digest := sha256.Sum256([]byte(value.SessionID + "|" + fmt.Sprint(value.SessionCreated) + "|" + value.SocketFingerprint))
	return hex.EncodeToString(digest[:])[:32]
}

func syncErrorStatus(err error) string {
	switch {
	case errors.Is(err, errRemoteStateMissing):
		return syncStatusMissing
	case errors.Is(err, errRemoteTmuxMissing):
		return syncStatusTmuxMissing
	case errors.Is(err, errRemoteStateInvalid):
		return syncStatusInvalid
	case errors.Is(err, ports.ErrTransportUnavailable):
		return syncStatusUnavailable
	default:
		return syncStatusUnavailable
	}
}

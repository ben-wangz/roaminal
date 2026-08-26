package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

func (s *Service) installRemote(ctx context.Context, operationID, id string, binary []byte, request installRequest) error {
	suffix := fmt.Sprintf("%x", sha256.Sum256([]byte(request.Endpoint.Key+request.ReplacementToken)))[:32]
	uploadScript := "set -eu\numask 077\n" +
		"mkdir -p \"$HOME/.roaminal\"\n" +
		"tmp=\"$HOME/.roaminal/.upload-$1\"\n" +
		"umask 077\ncat > \"$tmp\"\nchmod 0700 \"$tmp\"\nprintf '%s\\n' \"$tmp\"\n"
	uploadStarted := time.Now()
	logAgentInfo("agent_component_upload_started", operationID, id, "bytes=%d", len(binary))
	upload, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: uploadScript, Args: []string{suffix}, Stdin: bytes.NewReader(binary), Timeout: 10 * time.Second, OutputLimit: 4096})
	if err != nil || strings.TrimSpace(string(upload.Output)) == "" {
		s.cleanupRemote(ctx, id, suffix)
		if err == nil {
			err = errors.New("remote upload path was not returned")
		}
		logAgentInfo("agent_component_upload_failed", operationID, id, "duration_ms=%d error_type=%T", durationMillis(uploadStarted), err)
		return remoteAgentError("agent_install_failed", 502, "The Agent component upload failed.", err)
	}
	logAgentInfo("agent_component_upload_completed", operationID, id, "duration_ms=%d", durationMillis(uploadStarted))
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	installScript := "set +e\n" +
		"tmp=\"$HOME/.roaminal/.upload-$1\"\n" +
		"\"$tmp\" install\n" +
		"status=$?\nrm -f -- \"$tmp\"\nexit $status\n"
	installStarted := time.Now()
	logAgentInfo("agent_component_install_started", operationID, id, "component_version=%q component_sha256=%q", request.ComponentVersion, request.ComponentSHA256)
	result, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: installScript, Args: []string{suffix}, Stdin: bytes.NewReader(payload), Timeout: 15 * time.Second, OutputLimit: 8192})
	if err != nil {
		logAgentInfo("agent_component_install_failed", operationID, id, "duration_ms=%d error_type=%T", durationMillis(installStarted), err)
		s.cleanupRemote(ctx, id, suffix)
		if mapped := helperInstallError(result.ErrorOutput); mapped != nil {
			return mapped
		}
		return remoteAgentError("agent_install_failed", 502, "The remote Agent component installation failed.", err)
	}
	logAgentInfo("agent_component_install_completed", operationID, id, "duration_ms=%d", durationMillis(installStarted))
	var response installResponse
	if json.Unmarshal(result.Output, &response) != nil || response.EndpointKey != request.Endpoint.Key {
		return errf("agent_verification_failed", 502, "The installed Agent component could not be verified.", nil)
	}
	return nil
}

func helperInstallError(data []byte) error {
	var value struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	switch value.Error {
	case "endpoint_conflict":
		return errf("agent_endpoint_conflict", 409, "The remote Agent is bound to another SSH endpoint.", nil)
	case "binding_changed":
		return errf("agent_binding_conflict", 409, "The remote Agent binding changed during initialization.", nil)
	case "component downgrade":
		return errf("agent_install_failed", 409, "The remote Agent component cannot be downgraded.", nil)
	case "hooks file permissions are unsafe", "hooks must be an object", "hooks root must be an object":
		return errf("agent_hooks_invalid", 409, "The existing Codex Hooks configuration is invalid or unsafe.", nil)
	case "invalid install request", "invalid component checksum", "component checksum mismatch":
		return errf("agent_verification_failed", 502, "The installed Agent component could not be verified.", nil)
	default:
		return nil
	}
}

func (s *Service) cleanupRemote(ctx context.Context, id, suffix string) {
	_, _ = s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: "rm -f -- \"$HOME/.roaminal/.upload-$1\"\n", Args: []string{suffix}, Timeout: 3 * time.Second, OutputLimit: 256})
}

func (s *Service) verifyRemote(ctx context.Context, id, expectedFingerprint, endpointKey, componentSHA256 string) error {
	result, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: "set -eu\n\"$HOME/.roaminal/bin/roaminal-agent-hook\" probe\n", Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		return remoteAgentError("agent_verification_failed", 502, "The installed Agent component could not be verified.", err)
	}
	var response struct {
		TokenFingerprint string `json:"tokenFingerprint"`
		EndpointKey      string `json:"endpointKey"`
		ComponentSHA256  string `json:"componentSha256"`
		HooksConfigured  bool   `json:"hooksConfigured"`
	}
	if json.Unmarshal(result.Output, &response) != nil || response.TokenFingerprint != expectedFingerprint || response.EndpointKey != endpointKey || !response.HooksConfigured || response.ComponentSHA256 != componentSHA256 {
		return errf("agent_verification_failed", 502, "The installed Agent component could not be verified.", nil)
	}
	return nil
}

func remoteAgentError(code string, status int, message string, err error) error {
	if errors.Is(err, ports.ErrTransportUnavailable) {
		return errf("agent_transport_unavailable", 503, "The SSH transport became unavailable during Agent initialization.", err)
	}
	return errf(code, status, message, err)
}

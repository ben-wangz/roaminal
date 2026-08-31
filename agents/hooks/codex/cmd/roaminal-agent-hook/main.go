package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/buildinfo"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/hooks"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/report"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/state"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/tmux"
)

func main() {
	command := "version"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	var err error
	switch command {
	case "version":
		err = output(map[string]any{"schemaVersion": model.SchemaVersion, "componentVersion": buildinfo.Version, "binary": "roaminal-agent-hook", "os": runtime.GOOS, "arch": runtime.GOARCH})
	case "probe":
		err = probe()
	case "read-state":
		err = readState()
	case "install":
		err = install()
	case "hook":
		err = hook()
	default:
		err = errors.New("unknown command")
	}
	if err != nil {
		report.LogDiagnostic(homeDir(), "agent_command_failed", map[string]string{"command": command, "error": safeError(err)})
		failure := map[string]string{"error": safeError(err)}
		if command == "install" {
			failure["code"] = installErrorCode(err)
		}
		if command == "read-state" {
			failure["code"] = stateErrorCode(err)
		}
		_ = json.NewEncoder(os.Stderr).Encode(failure)
		os.Exit(1)
	}
}

func homeDir() string {
	if value := strings.TrimSpace(os.Getenv("HOME")); value != "" {
		return value
	}
	value, _ := os.UserHomeDir()
	return value
}

func configPath() string { return filepath.Join(homeDir(), ".roaminal", "agent.json") }

func readComponentConfig() (model.ComponentConfig, error) {
	data, err := readPrivateFile(configPath(), 64*1024)
	if err != nil {
		return model.ComponentConfig{}, err
	}
	var cfg model.ComponentConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return model.ComponentConfig{}, err
	}
	if cfg.FormatVersion != model.SchemaVersion || cfg.Provider != model.ProviderCodex || cfg.ComponentVersion == "" || !validChecksum(cfg.ComponentSHA256) {
		return model.ComponentConfig{}, errors.New("invalid component configuration")
	}
	return cfg, nil
}

func probe() error {
	config, configErr := readComponentConfig()
	client, tmuxErr := tmux.New()
	response := probeResponse(config, configErr, tmuxErr)
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		info, discoverErr := client.Discover(ctx)
		cancel()
		if discoverErr == nil {
			response["tmux"] = info
			response["runtimeId"] = tmux.RuntimeID(info)
		}
	}
	return output(response)
}

func probeResponse(config model.ComponentConfig, configErr, tmuxErr error) map[string]any {
	response := map[string]any{
		"schemaVersion": model.SchemaVersion, "componentVersion": buildinfo.Version,
		"provider": model.ProviderCodex,
		"os":       runtime.GOOS, "arch": runtime.GOARCH, "tmuxAvailable": tmuxErr == nil,
		"codexConfig":     fileExists(filepath.Join(homeDir(), ".codex", "hooks.json")),
		"hooksConfigured": hooks.Configured(homeDir()), "configured": configErr == nil,
		"capabilities": model.StateCapabilities{Running: true, Relax: true, Error: false},
	}
	if configErr == nil {
		response["componentVersion"] = config.ComponentVersion
		response["componentSha256"] = config.ComponentSHA256
	}
	return response
}

func readState() error {
	if len(os.Args) != 4 || os.Args[2] != "--session" || strings.TrimSpace(os.Args[3]) == "" {
		return errors.New("read-state requires --session")
	}
	client, err := tmux.New()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info, err := client.DiscoverSession(ctx, os.Args[3])
	if err != nil {
		return fmt.Errorf("tmux session unavailable: %w", err)
	}
	file, err := state.Read(homeDir(), info)
	if err != nil {
		return err
	}
	return output(file)
}

func hook() error {
	home := homeDir()
	input, err := report.ReadInput(os.Stdin)
	if err != nil {
		report.LogDiagnostic(home, "hook_input_failed", map[string]string{"error": safeError(err)})
		return output(map[string]any{})
	}
	eventName := input["hook_event_name"]
	if !report.KnownEvent(eventName) {
		report.LogDiagnostic(home, "hook_event_ignored", map[string]string{"event_name": eventName})
		return output(map[string]any{})
	}
	report.LogDiagnostic(home, "hook_started", map[string]string{"event_name": eventName})
	client, err := tmux.New()
	if err != nil {
		report.LogDiagnostic(home, "hook_tmux_unavailable", map[string]string{"event_name": eventName, "error": safeError(err)})
		return output(map[string]any{})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	info, err := client.Discover(ctx)
	cancel()
	if err != nil {
		report.LogDiagnostic(home, "hook_tmux_discovery_failed", map[string]string{"event_name": eventName, "error": safeError(err)})
		return output(map[string]any{})
	}
	report.LogDiagnostic(home, "hook_tmux_discovered", map[string]string{
		"event_name": eventName, "session_name": info.SessionName, "session_id": info.SessionID,
		"session_created": formatInt(info.SessionCreated), "pane_id": info.PaneID,
		"socket_fingerprint": info.SocketFingerprint, "runtime_id": tmux.RuntimeID(info),
	})
	standardState, ok := report.StateFor(eventName, input["source"], input["reason"])
	if !ok {
		report.LogDiagnostic(home, "hook_event_ignored", map[string]string{"event_name": eventName})
		return output(map[string]any{})
	}
	record := model.StateRecord{
		Timestamp: time.Now().UTC(), State: standardState, EventName: eventName,
		Source: input["source"], Reason: input["reason"], ProviderSessionID: input["session_id"],
		TurnID: input["turn_id"], ToolUseID: input["tool_use_id"],
	}
	updated, err := state.Update(home, info, installedComponentVersion(home), record)
	if err != nil {
		report.LogDiagnostic(home, "hook_state_update_failed", map[string]string{
			"event_name": eventName, "state": standardState, "runtime_id": tmux.RuntimeID(info), "error": safeError(err),
		})
		return output(map[string]any{})
	}
	report.LogDiagnostic(home, "hook_state_updated", map[string]string{
		"event_name": eventName, "state": standardState, "runtime_id": updated.RuntimeID,
		"index": formatUint(updated.LatestIndex),
	})
	return output(map[string]any{})
}

func installedComponentVersion(home string) string {
	config, err := readPrivateComponentConfig(home)
	if err == nil && config.ComponentVersion != "" {
		return config.ComponentVersion
	}
	return buildinfo.Version
}

func readPrivateComponentConfig(home string) (model.ComponentConfig, error) {
	data, err := readPrivateFile(filepath.Join(home, ".roaminal", "agent.json"), 64*1024)
	if err != nil {
		return model.ComponentConfig{}, err
	}
	var config model.ComponentConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || config.Provider != model.ProviderCodex || config.FormatVersion != model.SchemaVersion {
		return model.ComponentConfig{}, errors.New("invalid component configuration")
	}
	return config, nil
}

func stateErrorCode(err error) string {
	if strings.Contains(strings.ToLower(err.Error()), "tmux session") {
		return "tmux_session_missing"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "state_missing"
	}
	if errors.Is(err, state.ErrRuntimeMismatch) {
		return "state_runtime_mismatch"
	}
	if errors.Is(err, state.ErrInvalidState) {
		return "state_invalid"
	}
	return "state_read_failed"
}

func formatInt(value int64) string   { return strconv.FormatInt(value, 10) }
func formatUint(value uint64) string { return strconv.FormatUint(value, 10) }

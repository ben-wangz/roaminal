package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/buildinfo"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/hooks"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/report"
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
	case "install":
		err = install()
	case "hook":
		err = hook()
	default:
		err = errors.New("unknown command")
	}
	if err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": safeError(err)})
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

func readConfig() (model.Config, error) {
	data, err := readPrivateFile(configPath(), 64*1024)
	if err != nil {
		return model.Config{}, err
	}
	var cfg model.Config
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return model.Config{}, err
	}
	fingerprint := tokenFingerprint(cfg.Token)
	if cfg.FormatVersion != model.SchemaVersion || cfg.AgentType != "codex" || cfg.Endpoint.Key == "" || cfg.WebhookURL == "" ||
		cfg.Token == "" || fingerprint == "" || fingerprint != cfg.TokenFingerprint ||
		cfg.ComponentVersion == "" || cfg.ComponentSHA256 == "" {
		return model.Config{}, errors.New("invalid agent configuration")
	}
	return cfg, nil
}

func install() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return errors.New("install input too large")
	}
	var request model.InstallRequest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	if request.SchemaVersion != model.SchemaVersion || request.Endpoint.Key == "" || request.WebhookURL == "" || request.ComponentVersion == "" || request.ComponentSHA256 == "" || request.TmuxSessionName == "" {
		return errors.New("invalid install request")
	}
	if !validChecksum(request.ComponentSHA256) {
		return errors.New("invalid component checksum")
	}
	if request.ReplacementToken != "" {
		if raw, err := decodeToken(request.ReplacementToken); err != nil || len(raw) != 32 {
			return errors.New("invalid replacement token")
		}
	}
	root := filepath.Join(homeDir(), ".roaminal")
	if err := ensurePrivateDir(root); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Join(root, "bin")); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Join(root, "locks")); err != nil {
		return err
	}
	lock, err := acquireLock(filepath.Join(root, "locks", "install.lock"), 30*time.Second)
	if err != nil {
		return err
	}
	defer lock()
	current, currentErr := readConfig()
	if currentErr == nil {
		if request.ExpectedCurrentTokenFingerprint == "" {
			return errors.New("binding_changed")
		}
		if current.Endpoint.Key != request.Endpoint.Key {
			return errors.New("endpoint_conflict")
		}
		if current.TokenFingerprint != request.ExpectedCurrentTokenFingerprint {
			return errors.New("binding_changed")
		}
	} else if !os.IsNotExist(currentErr) {
		return currentErr
	} else if request.ExpectedCurrentTokenFingerprint != "" {
		return errors.New("binding_changed")
	}
	if currentErr == nil && current.ComponentVersion != "" && componentVersionGreater(current.ComponentVersion, request.ComponentVersion) {
		return errors.New("component downgrade")
	}
	if checksum, checksumErr := executableChecksum(os.Args[0]); checksumErr != nil || checksum != request.ComponentSHA256 {
		return errors.New("component checksum mismatch")
	}
	if err := installBinaryIfNeeded(filepath.Join(root, "bin", "roaminal-agent-hook"), request.ComponentSHA256); err != nil {
		return err
	}
	if err := ensurePrivateDir(filepath.Join(homeDir(), ".codex")); err != nil {
		return err
	}
	if err := hooks.InstallHooks(homeDir()); err != nil {
		return err
	}
	token := request.ReplacementToken
	if token == "" && currentErr == nil {
		token = current.Token
	}
	if token == "" {
		return errors.New("replacement token is required")
	}
	fingerprint := tokenFingerprint(token)
	cfg := model.Config{
		FormatVersion: model.SchemaVersion, AgentType: "codex", Endpoint: request.Endpoint, WebhookURL: request.WebhookURL,
		Token: token, TokenFingerprint: fingerprint, ComponentVersion: request.ComponentVersion,
		ComponentSHA256: request.ComponentSHA256, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeConfig(cfg); err != nil {
		return err
	}
	return output(map[string]any{
		"schemaVersion": model.SchemaVersion, "result": "installed",
		"changed":          currentErr != nil || request.ReplacementToken != "",
		"componentVersion": cfg.ComponentVersion, "componentSha256": cfg.ComponentSHA256,
		"tokenFingerprint": fingerprint, "endpointKey": cfg.Endpoint.Key,
		"hooks": "configured", "needsTrust": true,
	})
}

func probe() error {
	cfg, err := readConfig()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	client, tmuxErr := tmux.New()
	response := model.ProbeResponse{
		SchemaVersion: model.SchemaVersion, ComponentVersion: buildinfo.Version,
		OS: runtime.GOOS, Arch: runtime.GOARCH, TmuxAvailable: tmuxErr == nil,
		CodexConfig:     fileExists(filepath.Join(homeDir(), ".codex", "hooks.json")),
		HooksConfigured: hooks.Configured(homeDir()),
	}
	if err == nil {
		response.TokenFingerprint = cfg.TokenFingerprint
		response.EndpointKey = cfg.Endpoint.Key
		response.ComponentSHA256 = cfg.ComponentSHA256
	}
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		info, discoverErr := client.Discover(ctx)
		cancel()
		if discoverErr == nil {
			return output(map[string]any{
				"schemaVersion": response.SchemaVersion, "componentVersion": response.ComponentVersion,
				"componentSha256": response.ComponentSHA256, "endpointKey": response.EndpointKey,
				"os": response.OS, "arch": response.Arch, "tmuxAvailable": response.TmuxAvailable,
				"codexConfig": response.CodexConfig, "hooksConfigured": response.HooksConfigured,
				"tokenFingerprint": response.TokenFingerprint, "tmux": info,
			})
		}
	}
	return output(response)
}

func hook() error {
	input, err := report.ReadInput(os.Stdin)
	if err != nil {
		return output(map[string]any{})
	}
	if !report.KnownEvent(input["hook_event_name"]) {
		return output(map[string]any{})
	}
	cfg, err := readConfig()
	if err != nil {
		return output(map[string]any{})
	}
	client, err := tmux.New()
	if err != nil {
		return output(map[string]any{})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	info, err := client.Discover(ctx)
	if err != nil {
		cancel()
		return output(map[string]any{})
	}
	claimed, err := client.Claim(ctx, info, input["session_id"])
	if err != nil || !claimed {
		cancel()
		return output(map[string]any{})
	}
	sequence, err := client.NextSequence(ctx, info)
	if err != nil {
		cancel()
		return output(map[string]any{})
	}
	event := report.NewEvent(input, info, cfg.Endpoint.Key, cfg.ComponentVersion, sequence)
	if err := report.WriteSpool(homeDir(), event, info); err != nil {
		cancel()
		return output(map[string]any{})
	}
	report.Drain(ctx, cfg, homeDir(), info)
	if input["hook_event_name"] == "SessionEnd" {
		_ = client.Release(ctx, info, input["session_id"])
	}
	cancel()
	return output(map[string]any{})
}

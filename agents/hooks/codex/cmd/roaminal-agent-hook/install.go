package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/hooks"
	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/model"
)

func install() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return errors.New("install input too large")
	}
	var request model.LocalInstallRequest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	if request.SchemaVersion != model.SchemaVersion || request.ComponentVersion == "" || !validChecksum(request.ComponentSHA256) {
		return errors.New("invalid install request")
	}
	root := filepath.Join(homeDir(), ".roaminal")
	for _, directory := range []string{
		root, filepath.Join(root, "bin"), filepath.Join(root, "locks"),
		filepath.Join(root, "state"), filepath.Join(root, "state", "agents"),
		filepath.Join(root, "state", "agents", model.ProviderCodex), filepath.Join(root, "logs"),
	} {
		if err := ensurePrivateDir(directory); err != nil {
			return err
		}
	}
	if err := ensurePrivateDir(filepath.Join(homeDir(), ".codex")); err != nil {
		return err
	}
	lock, err := acquireLock(filepath.Join(root, "locks", "install.lock"), 30*time.Second)
	if err != nil {
		return err
	}
	defer lock()

	current, currentErr := readComponentConfig()
	if currentErr == nil && componentVersionGreater(current.ComponentVersion, request.ComponentVersion) {
		return errors.New("component downgrade")
	}
	if checksum, checksumErr := executableChecksum(os.Args[0]); checksumErr != nil || checksum != request.ComponentSHA256 {
		return errors.New("component checksum mismatch")
	}
	if err := installBinaryIfNeeded(filepath.Join(root, "bin", "roaminal-agent-hook"), request.ComponentSHA256); err != nil {
		return err
	}
	if err := hooks.InstallHooks(homeDir()); err != nil {
		return err
	}
	config := model.ComponentConfig{
		FormatVersion: model.SchemaVersion, Provider: model.ProviderCodex,
		ComponentVersion: request.ComponentVersion, ComponentSHA256: request.ComponentSHA256,
		InstalledAt: time.Now().UTC(),
	}
	if err := writeComponentConfig(config); err != nil {
		return err
	}
	return output(map[string]any{
		"schemaVersion": model.SchemaVersion, "result": "installed",
		"changed":          currentErr != nil || current.ComponentVersion != request.ComponentVersion || current.ComponentSHA256 != request.ComponentSHA256,
		"componentVersion": config.ComponentVersion, "componentSha256": config.ComponentSHA256,
		"provider": model.ProviderCodex, "hooks": "configured", "needsTrust": true,
		"capabilities": model.StateCapabilities{Running: true, Relax: true, Error: false},
	})
}

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type assetManifest struct {
	BundleSchemaVersion int           `json:"bundleSchemaVersion"`
	ComponentVersion    string        `json:"componentVersion"`
	BinaryName          string        `json:"binaryName"`
	HooksConfig         assetDigest   `json:"hooksConfig"`
	Targets             []assetTarget `json:"targets"`
}
type assetDigest struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type assetTarget struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (s *Service) loadAsset(remoteOS, remoteArch string) (assetTarget, []byte, assetManifest, error) {
	root := filepath.Join(s.cfg.AgentHooksDir, "codex")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return assetTarget{}, nil, assetManifest{}, errf("agent_assets_unavailable", 503, "Agent component assets are unavailable.", err)
	}
	var manifest assetManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.ComponentVersion == "" || manifest.BinaryName == "" {
		return assetTarget{}, nil, assetManifest{}, errf("agent_assets_unavailable", 503, "Agent component assets are invalid.", err)
	}
	if !validTargetOSArch(remoteOS, remoteArch) {
		return assetTarget{}, nil, manifest, errf("agent_remote_platform_unsupported", 409, "The remote platform is not supported.", nil)
	}
	targetName := remoteOS + "-" + remoteArch
	var selected *assetTarget
	for index := range manifest.Targets {
		if manifest.Targets[index].Target == targetName {
			if selected != nil {
				return assetTarget{}, nil, manifest, errf("agent_assets_unavailable", 503, "Agent component assets contain duplicate targets.", nil)
			}
			selected = &manifest.Targets[index]
		}
	}
	if selected == nil {
		return assetTarget{}, nil, manifest, errf("agent_assets_unavailable", 503, "The matching Agent component asset is unavailable.", nil)
	}
	if !safeAssetPath(selected.Path) {
		return assetTarget{}, nil, manifest, errf("agent_assets_unavailable", 503, "The Agent component manifest contains an unsafe path.", nil)
	}
	binaryPath := filepath.Join(root, selected.Path)
	binary, err := os.ReadFile(binaryPath)
	if err != nil || int64(len(binary)) != selected.Size || sha256Hex(binary) != selected.SHA256 {
		return assetTarget{}, nil, manifest, errf("agent_assets_unavailable", 503, "The Agent component asset failed checksum validation.", err)
	}
	configPath := filepath.Join(root, "config", "hooks.json")
	configData, err := os.ReadFile(configPath)
	if err != nil || int64(len(configData)) != manifest.HooksConfig.Size || sha256Hex(configData) != manifest.HooksConfig.SHA256 {
		return assetTarget{}, nil, manifest, errf("agent_assets_unavailable", 503, "The Agent hook configuration failed checksum validation.", err)
	}
	return *selected, binary, manifest, nil
}

func validTargetOSArch(osName, arch string) bool {
	if osName != "linux" && osName != "darwin" {
		return false
	}
	return arch == "amd64" || arch == "arm64"
}

func safeAssetPath(value string) bool {
	clean := filepath.Clean(value)
	return value != "" && clean == value && !filepath.IsAbs(value) && clean != "." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func (s *Service) remotePlatform(ctx context.Context, id string) (string, string, error) {
	script := "set -eu\n" +
		"printf 'os=%s\\n' \"$(uname -s | tr '[:upper:]' '[:lower:]')\"\n" +
		"printf 'arch=%s\\n' \"$(uname -m)\"\n" +
		"if command -v tmux >/dev/null 2>&1; then printf 'tmux=1\\n'; else printf 'tmux=0\\n'; fi\n" +
		"if command -v codex >/dev/null 2>&1; then printf 'codex=1\\n'; else printf 'codex=0\\n'; fi\n"
	result, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: script, Timeout: 8 * time.Second, OutputLimit: 4096})
	if err != nil {
		return "", "", remoteAgentError("agent_remote_probe_failed", 502, "The remote Agent probe failed.", err)
	}
	values := parseRemotePlatform(result.Output)
	if !values.Tmux {
		return "", "", errf("agent_tmux_unavailable", 409, "tmux is unavailable on the remote connection.", nil)
	}
	if !values.Codex {
		return "", "", errf("agent_codex_unavailable", 409, "Codex is unavailable on the remote connection.", nil)
	}
	if values.Arch == "x86_64" {
		values.Arch = "amd64"
	}
	if values.Arch == "aarch64" {
		values.Arch = "arm64"
	}
	if !validTargetOSArch(values.OS, values.Arch) {
		return "", "", errf("agent_remote_platform_unsupported", 409, "The remote platform is not supported.", nil)
	}
	return values.OS, values.Arch, nil
}

func parseRemotePlatform(data []byte) remotePlatformInfo {
	var result remotePlatformInfo
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "os":
			result.OS = value
		case "arch":
			result.Arch = value
		case "tmux":
			result.Tmux = value == "1"
		case "codex":
			result.Codex = value == "1"
		}
	}
	return result
}

func (s *Service) existingProbe(ctx context.Context, id string) (remoteProbe, error) {
	script := "set +e\n" +
		"if [ -x \"$HOME/.roaminal/bin/roaminal-agent-hook\" ]; then\n" +
		"  \"$HOME/.roaminal/bin/roaminal-agent-hook\" probe\n" +
		"elif [ -f \"$HOME/.roaminal/agent.json\" ]; then\n" +
		"  printf '__configured__\\n'\n" +
		"fi\n"
	result, err := s.terms.RunRemote(ctx, id, ports.RemoteCommand{Script: script, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		return remoteProbe{}, remoteAgentError("agent_remote_probe_failed", 502, "The remote Agent probe failed.", err)
	}
	var value remoteProbe
	if json.Unmarshal(result.Output, &value) == nil {
		value.Configured = value.Configured || value.ComponentSHA256 != ""
		return value, nil
	}
	if strings.Contains(string(result.Output), "__configured__") {
		return remoteProbe{Configured: true}, nil
	}
	return remoteProbe{}, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

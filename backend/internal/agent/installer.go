package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/connection"
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
	result, err := s.terms.RunRemote(ctx, id, connection.RemoteCommand{Script: script, Timeout: 8 * time.Second, OutputLimit: 4096})
	if err != nil {
		return "", "", errf("agent_remote_probe_failed", 502, "The remote Agent probe failed.", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(result.Output), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	if values["tmux"] != "1" {
		return "", "", errf("agent_tmux_unavailable", 409, "tmux is unavailable on the remote connection.", nil)
	}
	if values["codex"] != "1" {
		return "", "", errf("agent_codex_unavailable", 409, "Codex is unavailable on the remote connection.", nil)
	}
	if values["arch"] == "x86_64" {
		values["arch"] = "amd64"
	}
	if values["arch"] == "aarch64" {
		values["arch"] = "arm64"
	}
	if !validTargetOSArch(values["os"], values["arch"]) {
		return "", "", errf("agent_remote_platform_unsupported", 409, "The remote platform is not supported.", nil)
	}
	return values["os"], values["arch"], nil
}

func (s *Service) existingProbe(ctx context.Context, id string) (remoteProbe, error) {
	script := "set +e\n" +
		"if [ -x \"$HOME/.roaminal/bin/roaminal-agent-hook\" ]; then\n" +
		"  \"$HOME/.roaminal/bin/roaminal-agent-hook\" probe\n" +
		"elif [ -f \"$HOME/.roaminal/agent.json\" ]; then\n" +
		"  printf '__configured__\\n'\n" +
		"fi\n"
	result, err := s.terms.RunRemote(ctx, id, connection.RemoteCommand{Script: script, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		return remoteProbe{}, errf("agent_remote_probe_failed", 502, "The remote Agent probe failed.", err)
	}
	var value remoteProbe
	if json.Unmarshal(result.Output, &value) == nil {
		value.Configured = value.TokenFingerprint != ""
		return value, nil
	}
	if strings.Contains(string(result.Output), "__configured__") {
		return remoteProbe{Configured: true}, nil
	}
	return remoteProbe{}, nil
}

func (s *Service) installRemote(ctx context.Context, id string, binary []byte, request installRequest) error {
	suffix := fmt.Sprintf("%x", sha256.Sum256([]byte(request.Endpoint.Key+request.ReplacementToken)))[:32]
	uploadScript := "set -eu\numask 077\n" +
		"mkdir -p \"$HOME/.roaminal\"\n" +
		"tmp=\"$HOME/.roaminal/.upload-$1\"\n" +
		"umask 077\ncat > \"$tmp\"\nchmod 0700 \"$tmp\"\nprintf '%s\\n' \"$tmp\"\n"
	upload, err := s.terms.RunRemote(ctx, id, connection.RemoteCommand{Script: uploadScript, Args: []string{suffix}, Stdin: bytes.NewReader(binary), Timeout: 10 * time.Second, OutputLimit: 4096})
	if err != nil || strings.TrimSpace(string(upload.Output)) == "" {
		s.cleanupRemote(ctx, id, suffix)
		return errf("agent_install_failed", 502, "The Agent component upload failed.", err)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	installScript := "set +e\n" +
		"tmp=\"$HOME/.roaminal/.upload-$1\"\n" +
		"\"$tmp\" install\n" +
		"status=$?\nrm -f -- \"$tmp\"\nexit $status\n"
	result, err := s.terms.RunRemote(ctx, id, connection.RemoteCommand{Script: installScript, Args: []string{suffix}, Stdin: bytes.NewReader(payload), Timeout: 15 * time.Second, OutputLimit: 8192})
	if err != nil {
		s.cleanupRemote(ctx, id, suffix)
		if mapped := helperInstallError(result.ErrorOutput); mapped != nil {
			return mapped
		}
		return errf("agent_install_failed", 502, "The remote Agent component installation failed.", err)
	}
	var response map[string]any
	if json.Unmarshal(result.Output, &response) != nil || response["endpointKey"] != request.Endpoint.Key {
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
	_, _ = s.terms.RunRemote(ctx, id, connection.RemoteCommand{Script: "rm -f -- \"$HOME/.roaminal/.upload-$1\"\n", Args: []string{suffix}, Timeout: 3 * time.Second, OutputLimit: 256})
}

func (s *Service) verifyRemote(ctx context.Context, id, expectedFingerprint, endpointKey, componentSHA256 string) error {
	result, err := s.terms.RunRemote(ctx, id, connection.RemoteCommand{Script: "set -eu\n\"$HOME/.roaminal/bin/roaminal-agent-hook\" probe\n", Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		return errf("agent_verification_failed", 502, "The installed Agent component could not be verified.", err)
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

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

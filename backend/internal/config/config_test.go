package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEnvironmentAndBounds(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ROAMINAL_CWD", cwd)
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Setenv("ROAMINAL_PASSWORD", "test-password")
	t.Setenv("ROAMINAL_PORT", "19846")
	c, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 19846 || c.InitialCwd != cwd || c.AuthAccessTTL != 15*time.Minute || !c.ClientDiagnosticsEnabled {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestClientDiagnosticsCanBeDisabledByArgsAndEnvironment(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ROAMINAL_CWD", cwd)
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Setenv("ROAMINAL_PASSWORD", "secret")
	t.Setenv("ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED", "true")
	if err := os.Unsetenv("ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED"); err != nil {
		t.Fatal(err)
	}
	c, err := Load([]string{"--client-diagnostics=false"})
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientDiagnosticsEnabled {
		t.Fatal("expected CLI value to disable diagnostics")
	}
	c, err = Load([]string{"--client-diagnostics", "false"})
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientDiagnosticsEnabled {
		t.Fatal("expected separate CLI value to disable diagnostics")
	}
	if err := os.Setenv("ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED", "true"); err != nil {
		t.Fatal(err)
	}
	c, err = Load([]string{"--client-diagnostics=false"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.ClientDiagnosticsEnabled {
		t.Fatal("expected environment to override CLI")
	}
	if err := os.Setenv("ROAMINAL_CLIENT_DIAGNOSTICS_ENABLED", "invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid diagnostics boolean")
	}
}

func TestExplicitEmptyPasswordRejected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	t.Setenv("ROAMINAL_CWD", cwd)
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Setenv("ROAMINAL_PASSWORD", "")
	if _, err := Load(nil); err == nil {
		t.Fatal("expected empty password error")
	}
}

func TestTermsRequired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ROAMINAL_CWD", t.TempDir())
	t.Setenv("ROAMINAL_PASSWORD", "secret")
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "false")
	if _, err := Load(nil); err == nil {
		t.Fatal("expected terms error")
	}
}

func TestCanonicalConfigRejectsUnknownFields(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ROAMINAL_CWD", cwd)
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Chdir(cwd)
	if err := os.WriteFile(filepath.Join(cwd, "config.json"), []byte(`{"historyLimit":10}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(nil); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestAgentHooksDirectoryLoadsFromEnvironmentAndArguments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ROAMINAL_CWD", t.TempDir())
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Setenv("ROAMINAL_PASSWORD", "secret")
	t.Setenv("ROAMINAL_AGENT_HOOKS_DIR", "/tmp/agent-bundle")
	c, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.AgentHooksDir != "/tmp/agent-bundle" {
		t.Fatalf("unexpected agent config: %+v", c)
	}
	if err := os.Unsetenv("ROAMINAL_AGENT_HOOKS_DIR"); err != nil {
		t.Fatal(err)
	}
	c, err = Load([]string{"--agent-hooks-dir=/tmp/override-agent-bundle"})
	if err != nil {
		t.Fatal(err)
	}
	if c.AgentHooksDir != "/tmp/override-agent-bundle" {
		t.Fatalf("unexpected CLI agent config: %+v", c)
	}
}

func TestWebPushVAPIDConfigurationIsLoadedAndValidated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ROAMINAL_CWD", t.TempDir())
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Setenv("ROAMINAL_PASSWORD", "secret")
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateBytes := make([]byte, 32)
	copy(privateBytes[32-len(private.D.Bytes()):], private.D.Bytes())
	t.Setenv("ROAMINAL_WEB_PUSH_VAPID_PUBLIC_KEY", base64.RawURLEncoding.EncodeToString(elliptic.Marshal(elliptic.P256(), private.X, private.Y)))
	t.Setenv("ROAMINAL_WEB_PUSH_VAPID_PRIVATE_KEY", base64.RawURLEncoding.EncodeToString(privateBytes))
	t.Setenv("ROAMINAL_WEB_PUSH_SUBJECT", "mailto:push@example.com")
	c, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.WebPushVAPIDPublicKey == "" || c.WebPushVAPIDPrivateKey == "" || c.WebPushSubject != "mailto:push@example.com" {
		t.Fatalf("unexpected Web Push configuration: %+v", c)
	}
	t.Setenv("ROAMINAL_WEB_PUSH_VAPID_PUBLIC_KEY", base64.RawURLEncoding.EncodeToString([]byte("invalid")))
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid VAPID public key error")
	}
}

func TestImagePreviewConfigurationLoadsFromEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ROAMINAL_CWD", t.TempDir())
	t.Setenv("ROAMINAL_ACCEPT_TERMS", "true")
	t.Setenv("ROAMINAL_PASSWORD", "secret")
	cacheDir, err := os.MkdirTemp("/var/tmp", "roaminal-config-image-preview-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheDir) })
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_DIR", cacheDir)
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_TARGET_MIB", "64")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_MAX_AGE", "2h")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CACHE_CLEANUP_INTERVAL", "5m")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_CONVERSIONS", "3")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_SOURCE_MIB", "8")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_OUTPUT_MIB", "4")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_STATIC_PIXELS", "123456")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_FRAMES", "12")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_MAX_ANIMATED_PIXELS", "654321")
	t.Setenv("ROAMINAL_FILESYSTEM_IMAGE_PREVIEW_CONVERSION_TIMEOUT", "12s")

	value, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if value.FilesystemImagePreviewCacheDir != cacheDir || value.FilesystemImagePreviewCacheTargetMiB != 64 || value.FilesystemImagePreviewCacheMaxAge != 2*time.Hour || value.FilesystemImagePreviewCacheCleanupInterval != 5*time.Minute || value.FilesystemImagePreviewMaxConversions != 3 || value.FilesystemImagePreviewMaxSourceMiB != 8 || value.FilesystemImagePreviewMaxOutputMiB != 4 || value.FilesystemImagePreviewMaxStaticPixels != 123456 || value.FilesystemImagePreviewMaxFrames != 12 || value.FilesystemImagePreviewMaxAnimatedPixels != 654321 || value.FilesystemImagePreviewConversionTimeout != 12*time.Second {
		t.Fatalf("unexpected image preview configuration: %+v", value)
	}
}

func TestImagePreviewCacheCannotOverlapReservedDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateRoot, err := os.MkdirTemp("/var/tmp", "roaminal-config-state-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateRoot) })
	workspaceRoot, err := os.MkdirTemp("/var/tmp", "roaminal-config-workspace-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workspaceRoot) })
	cacheRoot, err := os.MkdirTemp("/var/tmp", "roaminal-config-cache-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheRoot) })
	base := Config{
		FilesystemImagePreviewCacheDir:             filepath.Join(cacheRoot, "cache"),
		FilesystemImagePreviewCacheTargetMiB:       128,
		FilesystemImagePreviewCacheMaxAge:          24 * time.Hour,
		FilesystemImagePreviewCacheCleanupInterval: 10 * time.Minute,
		FilesystemImagePreviewMaxConversions:       1,
		FilesystemImagePreviewMaxSourceMiB:         32,
		FilesystemImagePreviewMaxOutputMiB:         16,
		FilesystemImagePreviewMaxStaticPixels:      100000000,
		FilesystemImagePreviewMaxFrames:            200,
		FilesystemImagePreviewMaxAnimatedPixels:    200000000,
		FilesystemImagePreviewConversionTimeout:    30 * time.Second,
		StateDir:                                   stateRoot,
		InitialCwd:                                 workspaceRoot,
	}
	if err := validateFilesystemImagePreview(base); err != nil {
		t.Fatalf("valid image preview configuration rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"cache inside state": func(value *Config) {
			value.FilesystemImagePreviewCacheDir = filepath.Join(stateRoot, "cache")
		},
		"state inside cache": func(value *Config) {
			value.StateDir = filepath.Join(cacheRoot, "cache", "state")
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if err := validateFilesystemImagePreview(value); err == nil {
				t.Fatal("expected overlapping cache and reserved directory to be rejected")
			}
		})
	}
}

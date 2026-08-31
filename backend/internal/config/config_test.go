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

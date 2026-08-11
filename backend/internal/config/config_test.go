package config

import (
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

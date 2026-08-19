package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/buildinfo"
)

// TestLoadConfigDefaults covers REQ-MT-013 defaults: no env vars and no .env
// file resolve to 127.0.0.1:3000 with the buildinfo version.
func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfigFrom(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Bind != "127.0.0.1" {
		t.Errorf("Bind = %q, want %q", cfg.Bind, "127.0.0.1")
	}
	if cfg.Port != "3000" {
		t.Errorf("Port = %q, want %q", cfg.Port, "3000")
	}
	if cfg.Version != buildinfo.Version {
		t.Errorf("Version = %q, want buildinfo.Version %q", cfg.Version, buildinfo.Version)
	}
}

// TestLoadConfigDotEnvTier covers the .env tier (ADR-0007 §D5): values from
// the file are used when no environment variable is set.
func TestLoadConfigDotEnvTier(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "# local overrides\nMCP_BIND=127.0.0.2\nMCP_PORT=4000\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfigFrom(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bind != "127.0.0.2" {
		t.Errorf("Bind = %q, want %q", cfg.Bind, "127.0.0.2")
	}
	if cfg.Port != "4000" {
		t.Errorf("Port = %q, want %q", cfg.Port, "4000")
	}
}

// TestLoadConfigEnvOverridesDotEnv covers the precedence rule env > .env
// (REQ-MT-013): the environment variable wins over the .env file.
func TestLoadConfigEnvOverridesDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MCP_BIND=127.0.0.2\nMCP_PORT=4000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCP_BIND", "127.0.0.3")
	t.Setenv("MCP_PORT", "5000")

	cfg, err := loadConfigFrom(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bind != "127.0.0.3" {
		t.Errorf("Bind = %q, want env value %q", cfg.Bind, "127.0.0.3")
	}
	if cfg.Port != "5000" {
		t.Errorf("Port = %q, want env value %q", cfg.Port, "5000")
	}
}

// TestLoadConfigEnvPartialOverride covers REQ-MT-013 "Custom port": only
// MCP_PORT is set, the bind keeps its .env value.
func TestLoadConfigEnvPartialOverride(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("MCP_BIND=127.0.0.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCP_PORT", "4000")

	cfg, err := loadConfigFrom(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bind != "127.0.0.2" {
		t.Errorf("Bind = %q, want .env value %q", cfg.Bind, "127.0.0.2")
	}
	if cfg.Port != "4000" {
		t.Errorf("Port = %q, want %q", cfg.Port, "4000")
	}
}

// TestLoadConfigMissingDotEnvIsSilent covers the common case where no .env
// file exists: defaults apply and no error surfaces.
func TestLoadConfigMissingDotEnvIsSilent(t *testing.T) {
	cfg, err := loadConfigFrom(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bind != "127.0.0.1" || cfg.Port != "3000" {
		t.Errorf("cfg = %+v, want defaults 127.0.0.1:3000", cfg)
	}
}

// TestLoadConfigDotEnvReadError covers the fail-fast path: a configured but
// unreadable .env file must surface an error instead of silently binding
// defaults.
func TestLoadConfigDotEnvReadError(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("MCP_BIND=127.0.0.2\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on windows")
	}

	if _, err := loadConfigFrom(envPath); err == nil {
		t.Fatal("loadConfigFrom() = nil error, want error for unreadable .env")
	}
}

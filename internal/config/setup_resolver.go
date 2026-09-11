package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ResolveSetupDir returns the directory that contains the setup wizard JSONs.
// It mirrors install.sh resolve_paths per OS, with MCP_SETUP_DIR taking
// precedence on every platform. Symlink checks are intentionally not mirrored
// at boot time (ADR-SD-2): the install-time safety gate already validated the
// directory, and the boot resolver is read-only and unattended.
func ResolveSetupDir() (string, error) {
	return resolveSetupDirOS(runtime.GOOS, os.Getenv)
}

// resolveSetupDirOS is the injectable core of ResolveSetupDir.
// goos and getenv are parameters so the per-OS table is fully unit-testable
// on any development OS without build tags.
func resolveSetupDirOS(goos string, getenv func(string) string) (string, error) {
	if d := getenv("MCP_SETUP_DIR"); d != "" {
		return filepath.Clean(d), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("no se puede resolver el directorio de setup: la variable HOME no está definida")
	}

	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "MCP Appointments CRM", "setup"), nil
	default:
		base := getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "mcp-appointments-crm", "setup"), nil
	}
}

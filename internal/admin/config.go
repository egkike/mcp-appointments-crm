// Package admin implements the local operator gateway of the admin TUI: the
// owner seed decision, the first-owner creation flow, and the caller-id file
// that `mcp-server hermes chat` reads (ADR-0016 Decision 1, ADR-0012).
//
// The package is deliberately framework-free. T2 lands the seed gateway, T3+
// land the account flows, and T7 assembles the Bubble Tea screens on top of
// these functions; no UI framework is imported here.
package admin

import (
	"os"
	"path/filepath"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

const (
	// CallerIDFileName is the file the Hermes chat reads to resolve the owner
	// caller id when MCP_CALLER_ID is unset (docs/architecture/0012-hermes-chat-local.md:22).
	CallerIDFileName = "caller-id"

	// ConfigDirEnvVar overrides the configuration directory, mirroring the
	// MCP_DB_PATH override pattern of cmd/mcp-server/main.go (D1).
	ConfigDirEnvVar = "MCP_CONFIG_DIR"

	// defaultConfigDirName is the directory under ~/.config that holds the
	// operator configuration (ADR-0012 §Default).
	defaultConfigDirName = "mcp-appointments-crm"

	// configDirMode and callerIDFileMode keep the caller id readable only by
	// the OS user that owns the installation: it is identity material, not
	// shared configuration.
	configDirMode    = 0o700
	callerIDFileMode = 0o600
)

// ConfigDir resolves the operator configuration directory: MCP_CONFIG_DIR when
// set, otherwise ~/.config/mcp-appointments-crm (ADR-0012 §Default).
func ConfigDir() (string, error) {
	if dir := os.Getenv(ConfigDirEnvVar); dir != "" {
		return filepath.Clean(dir), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fail secure: without a home directory there is no safe default
		// location for the caller id, and guessing one could write identity
		// material into an unexpected path.
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo resolver el directorio de configuración: no hay directorio home",
			Cause:   err,
		}
	}
	return filepath.Join(home, ".config", defaultConfigDirName), nil
}

// CallerIDPath returns the path of the caller-id file inside ConfigDir.
func CallerIDPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, CallerIDFileName), nil
}

// WriteCallerID writes callerID verbatim to <config-dir>/caller-id with 0600
// permissions and returns the path it wrote.
//
// It creates the configuration directory (0700) when missing and truncates any
// previous file, so a re-seed never leaves a stale id behind. The file is
// chmod-ed explicitly after the write: os.WriteFile only applies the mode when
// it creates the file, and an inherited loose mode must not survive.
//
// The returned error never leaks the path (security checklist: no internal
// paths in error messages). The caller prints the path itself when the write
// succeeded, because the operator legitimately needs to know where it landed.
func WriteCallerID(callerID string) (string, error) {
	if callerID == "" {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el caller id no puede estar vacío",
		}
	}

	path, err := CallerIDPath()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el directorio de configuración",
			Cause:   err,
		}
	}

	if err := os.WriteFile(path, []byte(callerID), callerIDFileMode); err != nil {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir el archivo caller-id",
			Cause:   err,
		}
	}

	if err := os.Chmod(path, callerIDFileMode); err != nil {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos del archivo caller-id",
			Cause:   err,
		}
	}

	return path, nil
}

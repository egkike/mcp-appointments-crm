//go:build !windows

package auth

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// ensureAuditHashKeyOwnerOnly is the POSIX access-control gate for
// <configDir>/audit-hash-key and its directory (ADR-0021, issue #155 gate).
//
// Both checks are byte-for-byte the checks that used to live inline in
// readAuditHashKeyFile; they moved behind auditHashKeyPermissionGate in #158 so
// the Windows build can enforce the same contract through ACLs instead of
// skipping it. Their semantics are unchanged:
//
//   - the key file must be exactly 0600: a readable HMAC key exposes the audit
//     identity scheme, and a writable one lets another user replace the whole
//     digest namespace;
//   - the config dir must not be GROUP/WORLD-writable (0o022). Group-writable
//     dirs are the normal Linux layout under umask 0002, so demanding 0700
//     there would brick regular single-host installs (probe: t.TempDir() yields
//     0755 here). What matters is that nobody else can REPLACE the key file by
//     delete + rewrite through the directory.
//
// A stat failure is not a verdict: ERR_NOT_EXIST reaches the create path
// (fs.ErrNotExist ⇒ skip), any other stat error fails the gate right here,
// and symlinks/ownership/non-regular files land their own rejections before
// the read. Test fixtures explicit-chmod 0700 because t.TempDir() yields 0775
// in this environment.
func auditHashKeyPlatformGate(path string, kind auditGateTarget) error {
	switch kind {
	case auditGateDir:
		return ensureAuditHashKeyDirOwnerOnly(path)
	default:
		return ensureAuditHashKeyOwnerOnly(path)
	}
}

// ensureAuditHashKeyDirOwnerOnly evaluates the DIRECTORY itself (mode/ownership/
// symlink). It is the create-path pre-gate target: the key file the fresh key
// inherits is ITS own ACL, so the grandparent never enters the verdict (a probe
// artifact of this environment: t.TempDir() is group-writable at 0775).
func ensureAuditHashKeyDirOwnerOnly(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		// First boot: MkdirAll creates the dir tight below; the file-level
		// gate re-evaluates after the chmod.
		return nil
	}
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo evaluar el directorio de configuración de la clave del digest de auditoría (no se asume que sea segura)",
			Cause:   err,
		}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el directorio de configuración es un symlink: el salto no se verifica en este gate",
			Cause:   fs.ErrPermission,
		}
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && uint32(os.Geteuid()) != st.Uid { //nolint:gosec // Uid is unsigned kernel-side; euid fits u32
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el directorio de configuración pertenece a otro usuario: el gate exige propiedad del usuario del proceso",
			Cause:   fs.ErrPermission,
		}
	}
	if info.Mode().Perm()&0o022 != 0 {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el directorio de configuración es escribible por terceros: corré chmod 0700 sobre el directorio de configuración y volvé a arrancar",
			Cause:   fs.ErrPermission,
		}
	}
	return nil
}

func ensureAuditHashKeyOwnerOnly(path string) error {
	// GGA round 2 of the #158 unit: Lstat everywhere (a symlink swapped into
	// the key path never resolves through the hop), plus the ownership check
	// the mode bits alone cannot answer — another user's 0600 file is either
	// unreadable by this process (immediate fail at ReadFile) or, if readable,
	// owned by the wrong principal and rejected here.
	info, lerr := os.Lstat(path)
	if lerr == nil && !info.Mode().IsRegular() {
		// GGA round 7 of #158: the gate requires a REGULAR file. A FIFO owned
		// by the process with mode 0600 otherwise passes the gate and then
		// blocks ReadFile (startup hang); a symlink was the prior class of
		// this same rejection (the hop must never be followed).
		if info.Mode()&os.ModeSymlink != 0 {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la clave del digest de auditoría es un symlink: el salto no se verifica en este gate; apoyó el archivo real y volvé a arrancar",
				Cause:   fs.ErrPermission,
			}
		}
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la clave del digest de auditoría no es un archivo regular; reemplazala por un archivo plano con 0600",
			Cause:   fs.ErrPermission,
		}
	}
	if lerr != nil && !errors.Is(lerr, fs.ErrNotExist) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo evaluar la clave del digest de auditoría (no se asume que sea segura)",
			Cause:   lerr,
		}
	}
	if lerr == nil {
		if info.Mode().Perm() != os.FileMode(auditHashKeyFileMode) {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la clave del digest de auditoría es legible por terceros: corré chmod 0600 y volvé a arrancar",
				Cause:   fs.ErrPermission,
			}
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && uint32(os.Geteuid()) != st.Uid { //nolint:gosec // Uid is unsigned kernel-side; euid fits u32
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la clave del digest de auditoría pertenece a otro usuario: el gate exige propiedad del usuario del proceso",
				Cause:   fs.ErrPermission,
			}
		}
		// Parent-dir gate (only when the config dir already exists — first
		// boot recreates it tight and the file inherits its tight ACL).
		rd, rderr := os.Lstat(filepath.Dir(path))
		switch {
		case rderr == nil && rd.Mode()&os.ModeSymlink != 0:
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el directorio de configuración es un symlink: el salto no se verifica en este gate",
				Cause:   fs.ErrPermission,
			}
		case rderr == nil:
			if st, ok := rd.Sys().(*syscall.Stat_t); ok && uint32(os.Geteuid()) != st.Uid { //nolint:gosec // Uid is unsigned kernel-side; euid fits u32
				return &domain.SemanticError{
					Code:    domain.ErrCodeInvalidInput,
					Message: "el directorio de configuración pertenece a otro usuario: el gate exige propiedad del usuario del proceso",
					Cause:   fs.ErrPermission,
				}
			}
			if rd.Mode().Perm()&0o022 != 0 {
				return &domain.SemanticError{
					Code:    domain.ErrCodeInvalidInput,
					Message: "el directorio de configuración es escribible por terceros: corré chmod 0700 sobre el directorio de configuración y volvé a arrancar",
					Cause:   fs.ErrPermission,
				}
			}
		case errors.Is(rderr, fs.ErrNotExist):
			// First boot on a fresh tree: handled by the dir gate.
		default:
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "no se pudo evaluar el directorio de configuración de la clave del digest de auditoría (no se asume que sea segura)",
				Cause:   rderr,
			}
		}
	}
	return nil
}

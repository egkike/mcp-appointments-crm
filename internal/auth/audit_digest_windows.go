//go:build windows

package auth

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"golang.org/x/sys/windows"
)

// Windows access-control gate for <configDir>/audit-hash-key (ADR-0021, #158).
//
// os.FileMode on Windows only carries the read-only attribute, so the POSIX
// mode-bit gate cannot express the owner-only contract there — it would reject
// every Windows install or accept every one, and the #158 regression was the
// second. This gate reads the EFFECTIVE security descriptor
// (GetNamedSecurityInfo on SE_FILE_OBJECT, DACL + OWNER) of the config dir and
// of the key file, then fails closed when an ACCESS_ALLOWED ACE grants a
// principal other than the installation user read or write access. Tolerated
// "install user" principals: the object owner itself plus the two SIDs every
// default user-profile ACL carries (SYSTEM, Administrators).
//
// Declared deviations from the #158 outline (deliberate, not silent): only the
// key file and its immediate config dir are evaluated (no walk of nested
// children, and NO ancestor chain above the config dir — a
// mutable-by-others ancestor is an OPERATOR deploy misconfiguration, not
// something this boot gate can whitelist; see ADR-0021 boundary paragraph
// revising the review precedent for the reason this belongs to a separate
// candidate); the owner's GROUP memberships are not expanded (this inspects the
// DACL, it does not run an access check against the owner's token); ACE types
// other than ACCESS_ALLOWED/ACCESS_DENIED fail closed as unsupported (object
// ACEs use a different layout); the create path IS gated now: the pre-write
// dir gate, the post-write file gate and the fresh-dir re-gate all run before
// the boot answers (first-boot windows covered in audit_digest.go).
const (
	// auditKeyACLReadMask is data/config read (READ_CONTROL excluded on
	// purpose, GGA round 10: reading the SECURITY DESCRIPTOR is not reading
	// the key bytes, so a descriptor-only grant is not key exposure).
	auditKeyACLReadMask = windows.ACCESS_MASK(windows.GENERIC_READ | windows.FILE_GENERIC_READ | windows.FILE_READ_DATA)
	// auditKeyACLWriteMask is data write/append, DAC rewrite, ownership takeover
	// and DELETE: delete + recreate through the config dir is a namespace
	// takeover, the same reasoning as the POSIX group-write dir gate.
	// FILE_DELETE_CHILD (0x40) is spelled out because x/sys does not export it.
	auditKeyACLWriteMask = windows.ACCESS_MASK(windows.FILE_GENERIC_WRITE | windows.GENERIC_WRITE |
		windows.GENERIC_ALL | windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE | 0x40)
)

// auditKeyACLExposure is the verdict for one object: which access ANOTHER
// principal holds, and who holds it.
type auditKeyACLExposure struct {
	Read  bool
	Write bool
	// Principal is the string SID of the first ACE that grants access.
	Principal string
	// Via overrides the default "a <principal> en un ACE ACCESS_ALLOWED"
	// rendering for verdicts that are not an ordinary ACE (absent DACL).
	Via string
}

// describe renders the operator-facing variant (object, permission, ACE type,
// principal) plus the icacls fix — the Windows counterpart of `chmod 0600`.
// The binary never rewrites ACLs itself.
//
// Path handling (GGA rounds 1 and 4): no absolute path may cross the
// operator-facing message (round 1), yet the icacls remediation is INCOMPLETE
// without a target and without removing other principals' explicit grants
// (round 4). The message therefore carries a `<ruta de la clave>` PLACEHOLDER
// the operator substitutes themselves, plus /remove:g so the command actually
// revokes foreign grants instead of only re-granting the current user.
func (e auditKeyACLExposure) describe(label, path string) string {
	_ = path // the exact path travels only in Cause (server log)
	grant := "lectura"
	switch {
	case e.Read && e.Write:
		grant = "lectura y escritura"
	case e.Write:
		grant = "escritura"
	}
	subject := e.Via
	if subject == "" {
		subject = fmt.Sprintf("a %s en un ACE ACCESS_ALLOWED", e.Principal)
	}
	// Remediation target follows the failing OBJECT (GGA round 5 finding):
	// the key file and the config dir are different ACL objects, so one
	// remediation obviously does not fix the other.
	target := "<ruta de la clave>"
	if strings.Contains(label, "directorio") {
		target = "<ruta del directorio de configuración>"
	}
	// /reset replaces the explicit part of the DACL with the inherited one —
	// dropping EVERY foreign grant the gate may report, not just the two
	// best-known SIDs (GGA round 6 finding 2); /grant:r then hands the
	// installation user its read+write back.
	remediation := fmt.Sprintf(
		"restringé el ACL con `icacls \"%s\" /reset && icacls \"%s\" /inheritance:r /grant:r \"%%USERNAME%%:F\"`",
		target, target)
	return fmt.Sprintf("%s del digest de auditoría concede %s %s; %s y volvé a arrancar",
		label, grant, subject, remediation)
}

// ensureAuditHashKeyOwnerOnly evaluates BOTH gates in one call (dir first, then
// file) and reports every failure together, so the operator fixes one combined
// message instead of one ACL per boot. Missing targets are skipped: that is the
// first-boot create path.
func auditHashKeyPlatformGate(path string, kind auditGateTarget) error {
	userSID, err := auditKeyWindowsUserSID()
	if err != nil {
		// No installation identity, no tolerance list: fail closed.
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo determinar el usuario del proceso para el gate de ACLs de la clave del digest de auditoría",
			Cause:   err,
		}
	}
	switch kind {
	case auditGateDir:
		return ensureAuditHashKeyDirOwnerOnly(path, userSID)
	default:
		return ensureAuditHashKeyOwnerOnly(path, userSID)
	}
}

// ensureAuditHashKeyOwnerOnly evaluates the FILE plus its parent (dir+file in
// one call, as the read-path contract demands).
func ensureAuditHashKeyOwnerOnly(path string, userSID *windows.SID) error {
	targets := []struct{ path, label string }{
		{filepath.Dir(path), "el directorio de configuración de la clave del digest de auditoría"},
		{path, "la clave del digest de auditoría"},
	}
	var problems []string
	for _, target := range targets { // evaluated from the dir, matching the POSIX ordering
		// Symlink/reparse parity with the POSIX gate (GGA round 5): Lstat, and
		// a reparse point is rejected instead of being followed and evaluated
		// at its target.
		fi, serr := os.Lstat(target.path)
		if serr == nil && fi.Mode()&os.ModeSymlink != 0 {
			return auditKeyACLEvaluationError(target.label, errors.New(
				"el objetivo es un symlink/reparse point: el salto no se verifica en este gate; apoyé el objeto real con icacls"))
		}
		if serr != nil {
			if errors.Is(serr, fs.ErrNotExist) {
				continue
			}
			return auditKeyACLEvaluationError(target.label, serr)
		}
		exposure, err := auditKeyPathExposure(target.path, userSID, false)
		if err != nil {
			return auditKeyACLEvaluationError(target.label, err)
		}
		if exposure.Read || exposure.Write {
			problems = append(problems, exposure.describe(target.label, target.path))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: strings.Join(problems, "; "),
		Cause:   fs.ErrPermission,
	}
}

// ensureAuditHashKeyDirOwnerOnly evaluates the DIRECTORY ITSELF only (the
// create-path pre-gate: the fresh key inherits the dir's ACL, so the
// grandparent never enters the verdict).
func ensureAuditHashKeyDirOwnerOnly(path string, userSID *windows.SID) error {
	targets := []struct{ path, label string }{
		{path, "el directorio de configuración de la clave del digest de auditoría"},
	}
	var problems []string
	for _, target := range targets {
		// Symlink/reparse parity with the POSIX gate (GGA round 5): Lstat, and
		// a reparse point is rejected instead of being followed and evaluated
		// at its target.
		fi, serr := os.Lstat(target.path)
		if serr == nil && fi.Mode()&os.ModeSymlink != 0 {
			return auditKeyACLEvaluationError(target.label, errors.New(
				"el objetivo es un symlink/reparse point: el salto no se verifica en este gate; apoyé el objeto real con icacls"))
		}
		if serr != nil {
			if errors.Is(serr, fs.ErrNotExist) {
				continue
			}
			return auditKeyACLEvaluationError(target.label, serr)
		}
		exposure, err := auditKeyPathExposure(target.path, userSID, true)
		if err != nil {
			return auditKeyACLEvaluationError(target.label, err)
		}
		if exposure.Read || exposure.Write {
			problems = append(problems, exposure.describe(target.label, target.path))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: strings.Join(problems, "; "),
		Cause:   fs.ErrPermission,
	}
}

// auditKeyACLEvaluationError is the fail-closed branch: "could not evaluate" is
// never "safe". The technical cause stays in Cause, so no Win32 error text
// reaches the operator-facing message.
func auditKeyACLEvaluationError(label string, cause error) error {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInternal,
		Message: "no se pudieron evaluar los permisos de " + label + " (no se asume que sea seguro)",
		Cause:   cause,
	}
}

// auditKeyPathExposure reads one object's SD and evaluates it.
func auditKeyPathExposure(path string, userSID *windows.SID, includeInheritOnly bool) (auditKeyACLExposure, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return auditKeyACLExposure{}, fmt.Errorf("GetNamedSecurityInfo: %w", err)
	}
	if sd == nil {
		return auditKeyACLExposure{}, errors.New("el objeto no expone security descriptor")
	}
	return auditKeySDExposure(sd, userSID, includeInheritOnly)
}

// auditKeySDExposure is the parse+evaluate core: it takes a self-relative
// security descriptor (GetNamedSecurityInfo in production, a canned SDDL
// fixture in tests) and reports the first non-owner access grant.
func auditKeySDExposure(sd *windows.SECURITY_DESCRIPTOR, userSID *windows.SID, includeInheritOnly bool) (auditKeyACLExposure, error) {
	owner, _, err := sd.Owner()
	if err != nil {
		return auditKeyACLExposure{}, fmt.Errorf("GetSecurityDescriptorOwner: %w", err)
	}
	if owner == nil || !owner.IsValid() {
		return auditKeyACLExposure{}, errors.New("security descriptor sin owner SID válido")
	}
	// GGA round 3 of the #158 unit: ownership CONFERs control over the DACL
	// (implicit READ_CONTROL + WRITE_DAC through OWNER_RIGHTS, whether or not
	// an ACE exists). A foreign owner is therefore an exposure on its own,
	// even before reading any ACE.
	if !auditKeySIDTolerated(owner, userSID) {
		return auditKeyACLExposure{
			Read:  true,
			Write: true,
			Via:   "el owner (" + auditKeySIDString(owner) + ") no es el usuario del proceso (control implícito sobre el DACL)",
		}, nil
	}
	// An absent DACL (GetSecurityDescriptorDacl reports ERROR_OBJECT_NOT_FOUND)
	// means full control for everyone, and x/sys cannot distinguish it
	// reliably from an empty list: neither may be read as "no ACEs, so safe".
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return auditKeyACLExposure{
			Read:  true,
			Write: true,
			Via:   "a cualquier usuario (DACL ausente: control total)",
		}, nil
	}

	exposure := auditKeyACLExposure{}
	// GGA round 10: ACE order matters. Canonical ACLs put denials first, so a
	// matching deny CLEARS what earlier allow ACEs granted; a deny after an
	// allow is a non-canonical ACL and is treated as ambiguous + fail-closed.
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return auditKeyACLExposure{}, fmt.Errorf("GetAce(%d): %w", index, err)
		}
		if ace == nil {
			return auditKeyACLExposure{}, fmt.Errorf("GetAce(%d): ACE nulo", index)
		}
		// ACCESS_DENIED_ACE shares the ACCESS_ALLOWED_ACE layout; other types
		// (object/callback ACEs) do not, so they must not be reinterpreted.
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			// Canonical order: a deny that comes after an allow means the ACL
			// was not built in canonical order — ambiguous → fail closed.
			if exposure.Read || exposure.Write {
				return auditKeyACLExposure{}, fmt.Errorf(
					"DACL no canónica (deny %d después de allow): evaluación ambigua (fail closed)", index)
			}
			continue // a deny ACE grants nothing itself canonical-order
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return auditKeyACLExposure{}, fmt.Errorf("ACE de tipo %d no soportado en el DACL (fail closed)", ace.Header.AceType)
		}
		// INHERIT_ONLY applies to CHILDREN, never to this object (GGA round 7):
		// counting its mask as an effective grant on the OBJECT would wrongly
		// reject safe ACLs — but a DIR pre-gate MUST count it, because its
		// mask is exactly what the fresh key file INHERITS (GGA round 11).
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 && !includeInheritOnly {
			continue
		}
		// #nosec G103 -- the ACE's SidStart field is the SID's first 4 bytes; casting
		// its address is the documented x/sys pattern for reading an ACE principal
		// (see windows/syscall_windows_test.go) and stays inside the DACL memory
		// owned by sd (runtime.KeepAlive below).
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if auditKeySIDTolerated(sid, userSID) {
			continue
		}
		if ace.Mask&auditKeyACLWriteMask != 0 {
			exposure.Write = true
		}
		if ace.Mask&auditKeyACLReadMask != 0 {
			exposure.Read = true
		}
		if (exposure.Read || exposure.Write) && exposure.Principal == "" {
			exposure.Principal = auditKeySIDString(sid)
		}
	}
	runtime.KeepAlive(sd) // dacl and every ace pointer live inside sd's memory
	return exposure, nil
}

// auditKeySIDTolerated reports whether an ACE principal is the installation
// user (the process token's user SID — NOT the object owner: a foreign owner
// could otherwise hide its own granted access under the owner exemption), or
// one of the two SIDs every default user-profile ACL carries (SYSTEM,
// Administrators — they already control the profile, so excluding them would
// reject every stock install).
func auditKeySIDTolerated(sid, userSID *windows.SID) bool {
	if sid == nil || !sid.IsValid() {
		return false
	}
	if userSID != nil && sid.Equals(userSID) {
		return true
	}
	return sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

// auditKeyWindowsUserSID resolves the running process token's user SID.
func auditKeyWindowsUserSID() (*windows.SID, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(windows.Handle(token))
	info, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	if info == nil || info.User.Sid == nil {
		return nil, errors.New("el token del proceso no expone un SID de usuario")
	}
	return info.User.Sid, nil
}

// auditKeySIDString prefers the SDDL form (S-1-5-18) over an account lookup: it
// never blocks on a domain controller and is what icacls accepts.
func auditKeySIDString(sid *windows.SID) string {
	if sddl := sid.String(); sddl != "" {
		return sddl
	}
	return "un principal sin SID representable"
}

//go:build windows

package auth

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The fixtures in this file are the ACL twin of the POSIX permission-gate tests:
// they pin the ACE decision (which principals, which masks, which ACE types)
// against canned self-relative security descriptors, so the verdict logic is
// asserted without touching a real filesystem ACL.
//
// Evidence boundary (#158): this file COMPILES for GOOS=windows and is asserted
// only there. Nothing in it — nor the GetNamedSecurityInfo call it complements —
// is executed by the Linux CI job this unit was validated on. That runtime gap
// is stated in ADR-0021 and in the issue's evidence row; it is not claimed as
// coverage.

// testAuditKeyUserSID is the fake INSTALLATION user ('current process' in the
// tolerance semantics): NOT a well-known SID, so only this exact SID carries
// the owner exemption.
var testAuditKeyUserSID = func() *windows.SID {
	// Constructor failure at init can't be recovered gracefully below the gate;
	// a nil here would remove the owner exemption entirely and FAIL every
	// tolerance fixture loudly, which is the fail-closed direction.
	sid, _ := windows.StringToSid("S-1-5-100-1")
	return sid
}()

// testAuditKeyExposure decodes an SDDL descriptor and returns the gate verdict.
func testAuditKeyExposure(t *testing.T, sddl string) auditKeyACLExposure {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString(%q): %v", sddl, err)
	}
	exposure, err := auditKeySDExposure(sd, testAuditKeyUserSID, false)
	if err != nil {
		t.Fatalf("auditKeySDExposure(%q): %v", sddl, err)
	}
	return exposure
}

// TestAuditKeySDExposure_AcceptsOwnerAndProfilePrincipals pins the tolerance
// set: the owner's own ACE (here BUILTIN\Users as the object owner — a principal
// this gate does NOT special-case, so its ACE must be tolerated on the strength
// of the owner comparison alone) plus SYSTEM and Administrators, which is what
// a default user-profile ACL looks like.
func TestAuditKeySDExposure_AcceptsOwnerAndProfilePrincipals(t *testing.T) {
	// Accept: the INSTALLATION user + SYSTEM + Administrators (fixture user is
	// the file owner owning its ACE, mirroring the shipped window).
	exposure := testAuditKeyExposure(t, "O:S-1-5-100-1D:(A;;FA;;;S-1-5-100-1)(A;;FA;;;SY)(A;;FA;;;BA)")
	if exposure.Read || exposure.Write {
		t.Fatalf("exposure = %+v; want no exposure for owner+SYSTEM+Administrators", exposure)
	}
}

// TestAuditKeySDExposure_RejectsForeignRead pins the read variant: an
// Everyone (S-1-1-0) FILE_GENERIC_READ ACE makes the HMAC key readable by any
// local principal, which is exactly the exposure the POSIX 0600 gate blocks.
func TestAuditKeySDExposure_RejectsForeignRead(t *testing.T) {
	// Everyone READ = foreign grant still rejected (owner is the fake user).
	exposure := testAuditKeyExposure(t, "O:S-1-5-100-1D:(A;;FR;;;WD)(A;;FA;;;SY)")
	if !exposure.Read {
		t.Fatal("Read = false; want the Everyone read ACE reported")
	}
	if exposure.Write {
		t.Errorf("Write = true for a read-only ACE; the verdict must name the read variant")
	}
	if exposure.Principal != "S-1-1-0" {
		t.Errorf("Principal = %q; want the offending ACE's SID S-1-1-0", exposure.Principal)
	}
}

// TestAuditKeySDExposure_RejectsForeignWrite pins the replace-namespace
// variant: FILE_GENERIC_WRITE/delete/DAC rights let another principal rewrite
// or delete-and-recreate the key file.
func TestAuditKeySDExposure_RejectsForeignWrite(t *testing.T) {
	for _, sddl := range []string{
		"O:S-1-5-100-1D:(A;;FW;;;WD)", // FILE_GENERIC_WRITE (fixture-user owner)
		"O:S-1-5-100-1D:(A;;FA;;;WD)", // FILE_ALL_ACCESS (fixture-user owner)
		"O:BUD:(A;;WD;;;WD)",          // bare WRITE_DAC
		"O:BUD:(A;;SD;;;WD)",          // bare DELETE
	} {
		exposure := testAuditKeyExposure(t, sddl)
		if !exposure.Write {
			t.Errorf("%s: Write = false; want the write/delete ACE reported (%+v)", sddl, exposure)
		}
	}
}

// TestAuditKeySDExposure_RejectsForeignFullControlOnDirectory covers the config
// dir variant, where the ACE carries the container/object inherit flags.
func TestAuditKeySDExposure_RejectsForeignFullControlOnDirectory(t *testing.T) {
	// Inherited full-control on the directory: foreign too.
	exposure := testAuditKeyExposure(t, "O:S-1-5-100-1D:(A;OICI;FA;;;WD)(A;;FA;;;SY)(A;;FA;;;BA)")
	if !exposure.Read || !exposure.Write {
		t.Fatalf("exposure = %+v; want read+write for an inherited Everyone full-control ACE", exposure)
	}
}

// TestAuditKeySDExposure_IgnoresDenyACEs: a deny ACE never grants access, so a
// DACL of deny-only ACEs for foreign principals adds no exposure.
func TestAuditKeySDExposure_IgnoresDenyACEs(t *testing.T) {
	// Deny ACE ignored (deny never grants); drop-in BU is owner side.
	exposure := testAuditKeyExposure(t, "O:S-1-5-100-1D:(D;;FA;;;WD)(A;;FA;;;S-1-5-100-1)")
	if exposure.Read || exposure.Write {
		t.Fatalf("exposure = %+v; want no exposure from a deny ACE", exposure)
	}
}

// TestAuditKeySDExposure_TreatsAbsentDACLAsExposure pins the fail-closed
// branch: a security descriptor with no DACL is "full control for everyone" in
// SD semantics, never "no ACEs, therefore safe". The message must name the
// absent DACL rather than a fictitious ACE.
func TestAuditKeySDExposure_TreatsAbsentDACLAsExposure(t *testing.T) {
	sd, err := windows.NewSecurityDescriptor()
	if err != nil {
		t.Fatalf("NewSecurityDescriptor: %v", err)
	}
	owner := testAuditKeyUserSID
	if owner == nil {
		t.Fatal("fixture user SID was nil (StringToSid failure at init)")
	}
	if err := sd.SetOwner(owner, false); err != nil {
		t.Fatalf("SetOwner: %v", err)
	}

	exposure, err := auditKeySDExposure(sd, testAuditKeyUserSID, false)
	if err != nil {
		t.Fatalf("auditKeySDExposure(sin DACL): %v", err)
	}
	if !exposure.Read || !exposure.Write {
		t.Fatalf("exposure = %+v; want read+write exposure for an absent DACL", exposure)
	}
	message := exposure.describe("la clave del digest de auditoría", "C:\\config\\audit-hash-key")
	if !strings.Contains(message, "DACL ausente") {
		t.Errorf("message = %q; want the absent-DACL variant named", message)
	}
}

// TestAuditKeyACLExposure_DescribeNamesVariant pins the operator message: it
// must name the object (file vs dir), the permission (read vs write), the ACE
// type, the offending principal and the icacls fix — the Windows counterpart of
// the POSIX "corré chmod 0600" message.
func TestAuditKeyACLExposure_DescribeNamesVariant(t *testing.T) {
	const absPath = "C:\\config\\audit-hash-key"
	// No absolute path may cross the operator-facing copy (GGA round 1, path
	// free); the remediation carries a PLACEHOLDER the operator substitutes
	// (GGA round 4: the icacls command must be complete).
	message := auditKeyACLExposure{Read: true, Write: true, Principal: "S-1-1-0"}.
		describe("la clave del digest de auditoría", absPath)
	if strings.Contains(message, absPath) {
		t.Errorf("message = %q; the absolute path leaked into the operator copy", message)
	}
	for _, want := range []string{`<ruta de la clave>`, "lectura y escritura", "ACE ACCESS_ALLOWED", "S-1-1-0", "icacls", "/reset"} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q; missing %q", message, want)
		}
	}

	readOnly := auditKeyACLExposure{Read: true, Principal: "S-1-1-0"}.
		describe("la clave del digest de auditoría", absPath)
	if !strings.Contains(readOnly, " concede lectura a ") {
		t.Errorf("read-only message = %q; want the read variant named", readOnly)
	}
	// The remediation TARGET follows the failing object (GGA round 5): the dir
	// case must point the operator at the config dir, never at the key file.
	dirMsg := auditKeyACLExposure{Read: true, Write: true, Principal: "S-1-1-0"}.
		describe("el directorio de configuración de la clave del digest de auditoría", "C:\\config")
	if strings.Contains(dirMsg, "<ruta de la clave>") {
		t.Errorf("dir message = %q; want the config-dir remediation target", dirMsg)
	}
	if !strings.Contains(dirMsg, "<ruta del directorio de configuración>") {
		t.Errorf("dir message = %q; want the config-dir placeholder", dirMsg)
	}
	if strings.Contains(dirMsg, "C:\\config") {
		t.Errorf("dir message = %q; the absolute path leaked", dirMsg)
	}
}

// TestEnsureAuditHashKeyOwnerOnly_SkipsMissingTargets covers first boot: with
// neither the config dir nor the key file existing there is no ACL to evaluate,
// and the create path is responsible for the initial layout.
func TestEnsureAuditHashKeyOwnerOnly_SkipsMissingTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", AuditHashKeyFileName)
	if err := ensureAuditHashKeyOwnerOnly(path, testAuditKeyUserSID); err != nil {
		t.Fatalf("ensureAuditHashKeyOwnerOnly(missing targets) = %v; want nil", err)
	}
	// The dir-kind gate evaluates only the config dir itself (never probes the
	// grandparent), which is the create-path pre-gate contract on this side.
	if err := ensureAuditHashKeyDirOwnerOnly(filepath.Join(t.TempDir(), "missing-config"), testAuditKeyUserSID); err != nil {
		t.Fatalf("ensureAuditHashKeyDirOwnerOnly(missing targets) = %v; want nil", err)
	}
}

// TestAuditKeySDExposure_ForeignOwnerIsExposure pins the GGA round-3 rule: an
// object whose OWNER is a foreign principal controls the DACL implicitly, so
// the gate reports read+write exposure naming that owner even before any ACE
// is inspected.
func TestAuditKeySDExposure_ForeignOwnerIsExposure(t *testing.T) {
	sddl := "O:S-1-5-100-2D:" // foreign owner, no DACL at all besides implicit rights
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("SecurityDescriptorFromString(%q): %v", sddl, err)
	}
	exposure, err := auditKeySDExposure(sd, testAuditKeyUserSID, false)
	if err != nil {
		t.Fatalf("auditKeySDExposure(%q): %v", sddl, err)
	}
	if !exposure.Read || !exposure.Write {
		t.Fatalf("exposure = %+v, want read+write for the foreign-owner implicit rights", exposure)
	}
	if !strings.Contains(exposure.Via, "control implícito sobre el DACL") {
		t.Errorf("Via = %q, want the foreign-owner implicit-control wording", exposure.Via)
	}
}

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// The audit identity scheme (ADR-0021): every caller reference the audit trail
// persists is HMAC-SHA-256 keyed with an installation-local random key.
//
// Before ADR-0021 the reference was an unkeyed SHA-256 over the caller id.
// Caller ids are low entropy (E.164 phones, emails — a ~10^10–10^11 space), so
// an unkeyed digest is enumerable offline: anyone holding the audit table could
// recover the phone of every actor. The keyed digest moves that capability to
// the holder of the key file, which stays on the host with the database.
//
// Shape compatibility is deliberate: the digest is still 64 lowercase hex
// characters, so the accounts_audit column, its sink-side validation and every
// reader stay untouched (mixed acceptance window, ADR-0021).
const (
	// AuditHashKeyFileName is the installation key file inside the operator
	// config dir. It lives next to `caller-id`: both are identity material of
	// the same installation (internal/admin.ConfigDir).
	AuditHashKeyFileName = "audit-hash-key"

	// AuditHashKeySize is the raw key length in bytes. 32 bytes is both the
	// HMAC-SHA-256 block-friendly key size and the full digest width, so no
	// KDF/expansion is needed.
	AuditHashKeySize = 32

	// AuditDigestScheme labels the scheme in boot telemetry and degradation
	// logs, so the operator can tell from a log line which digest is active.
	AuditDigestScheme = "hmac-sha256"

	// auditHashKeyHexLen is the on-disk length: 64 lowercase hex chars.
	auditHashKeyHexLen = 2 * AuditHashKeySize

	// auditHashKeyFileMode / auditHashKeyDirMode keep the key readable only by
	// the OS user that owns the installation. The file is created 0600 and
	// re-chmod-ed explicitly after the write (an inherited loose mode must not
	// survive); the config dir is created 0700 like the caller-id flow does.
	auditHashKeyFileMode = 0o600
	auditHashKeyDirMode  = 0o700

	// auditHashKeyReadAttempts / auditHashKeyReadBackoff bound the re-read when
	// this process loses the create race: the server unit and an operator TUI
	// can boot at the same time, and the loser must read the winner's key
	// instead of overwriting it (the key is the digest namespace).
	auditHashKeyReadAttempts = 5
	auditHashKeyReadBackoff  = 20 * time.Millisecond
)

// CallerIDDigester turns a caller id into the opaque actor reference the audit
// trail stores. Implementations must be safe for concurrent use and must never
// emit the raw id.
type CallerIDDigester func(id string) string

// auditGateTarget selects which object the permission gate evaluates. The
// gate contract per platform:
//   - gateDir: the DIRECTORY target is evaluated ITSELF (mode/ACL) — never its
//     grandparent, which is irrelevant to the inherited-into-the-shared-ACL
//     mating (the create-path pre-gate use case);
//   - gateFile: the FILE plus its immediate parent dir are evaluated BOTH.
type auditGateTarget int

const (
	// auditGateDir pins the "existing config dir" pre-write evaluation.
	auditGateDir auditGateTarget = iota
	// auditGateFile pins the read/key evaluation (file + parent).
	auditGateFile
)

// auditHashKeyPermissionGate is the platform access-control gate applied to the
// key file and its config dir on every read (ADR-0021, issue #158).
//
// It is a variable, not a plain call, because the check is the one platform-
// specific piece of this protocol and the read path must be provably routed
// through it: the #158 regression was the Windows build skipping enforcement
// entirely while the POSIX mode bits were evaluated inline. POSIX evaluates
// mode bits (audit_digest_other.go); Windows evaluates the file/dir DACL
// (audit_digest_windows.go).
var auditHashKeyPermissionGate = auditHashKeyPlatformGate

// auditDigest is the process-wide keyed digest installed once at boot by the
// composition root (auth.ConfigureAuditDigest). It is nil until configured, and
// the read path treats nil as "not configured" — it never falls back to the
// retired unkeyed digest.
//
// The mutex is not protecting a hot race: ConfigureAuditDigest runs before any
// request or command goroutine starts. It exists so the read path never assumes
// that ordering, and so the configured digest is published safely.
var (
	auditDigestMu sync.RWMutex
	auditDigest   CallerIDDigester
)

// ConfigureAuditDigest installs the keyed caller-id digest for the process. It
// is the boot seam of ADR-0021: the composition root loads (or creates) the key
// file and calls this once, before dispatching any command, so serve mode and
// every sub-command share one digest namespace.
//
// A key that is not exactly AuditHashKeySize raw bytes is rejected: guessing or
// padding a short key silently would weaken the MAC, so the boot fails instead.
func ConfigureAuditDigest(key []byte) error {
	digester, err := NewCallerIDDigester(key)
	if err != nil {
		return err
	}
	auditDigestMu.Lock()
	defer auditDigestMu.Unlock()
	auditDigest = digester
	return nil
}

// AuditDigestConfigured reports whether a keyed digest is installed. Callers
// use it to distinguish "no audit reference available" from a real digest;
// production always answers true after the composition root configured it.
func AuditDigestConfigured() bool {
	auditDigestMu.RLock()
	defer auditDigestMu.RUnlock()
	return auditDigest != nil
}

// HashCallerID returns the keyed HMAC-SHA-256 digest of the caller id for audit
// and error logs: a correlation key that keeps raw PII (phone numbers, emails)
// out of log output and out of the durable audit rows (ADR-0021).
//
// The output is 64 lowercase hex chars — byte-for-byte the shape the retired
// unkeyed SHA-256 digest produced, so rows written under both schemes coexist
// unmodified in accounts_audit.
//
// Fail closed: when no digest is configured it returns "" and never degrades to
// the unkeyed digest. Callers must treat "" as "reference unavailable", omit the
// reference, and log the degradation (see AuthMiddleware.auditAttrs and
// AccountsRepo.newAccountAuditEvent); "" is also the empty actor_ref shape the
// audit sink already accepts (append-only rows are never dropped because a
// reference could not be computed).
func HashCallerID(id string) string {
	auditDigestMu.RLock()
	digester := auditDigest
	auditDigestMu.RUnlock()

	if digester == nil {
		return ""
	}
	return digester(id)
}

// NewCallerIDDigester builds the keyed digest for key. The key is copied, so a
// caller zeroing its slice afterwards cannot silently change the installed
// digest namespace.
//
// The returned closure is safe for concurrent use: hmac.New per call owns its
// own hash state (an hmac.Hash instance is stateful and must not be shared).
func NewCallerIDDigester(key []byte) (CallerIDDigester, error) {
	if len(key) != AuditHashKeySize {
		return nil, &domain.SemanticError{
			Code: domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf(
				"la clave del digest de auditoría debe tener %d bytes, se recibieron %d",
				AuditHashKeySize, len(key)),
		}
	}

	keyCopy := make([]byte, AuditHashKeySize)
	copy(keyCopy, key)

	return func(id string) string {
		mac := hmac.New(sha256.New, keyCopy)
		// hash.Hash.Write never returns an error; the nil check keeps errcheck
		// honest without an impossible branch.
		_, _ = mac.Write([]byte(id))
		return hex.EncodeToString(mac.Sum(nil))
	}, nil
}

// LoadOrCreateAuditHashKey returns the installation key from
// <configDir>/audit-hash-key, creating it on first use.
//
// Creation is the only write this function performs: 32 raw bytes from
// crypto/rand, hex-encoded on disk as 64 lowercase chars plus a trailing
// newline, file 0600 inside a 0700 directory. The file is opened with O_EXCL so
// two processes booting at once cannot each install a different key: the loser
// reads the winner's file (that is why the key is the digest namespace and must
// never be regenerated on an existing installation).
//
// Fail closed: an unreadable directory, an unreadable file, a malformed key and
// a key that cannot be created are all semantic errors — there is no fallback
// key and no in-memory-only mode, because either would silently break the
// audit trail's correlation.
func LoadOrCreateAuditHashKey(configDir string) ([]byte, error) {
	if strings.TrimSpace(configDir) == "" {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo resolver el directorio de configuración de la clave del digest de auditoría",
		}
	}
	return loadOrCreateAuditHashKeyFile(filepath.Join(configDir, AuditHashKeyFileName))
}

// loadOrCreateAuditHashKeyFile is the path-explicit core of
// LoadOrCreateAuditHashKey: the config dir is the only variable part, so the
// file protocol is testable against a throwaway directory.
func loadOrCreateAuditHashKeyFile(path string) ([]byte, error) {
	key, err := readAuditHashKeyFile(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return createAuditHashKeyFile(path)
}

// readAuditHashKeyFile reads and parses one key file. A missing file surfaces
// as fs.ErrNotExist (through the semantic error's cause) so the caller can
// decide to create it.
//
// Access-control gate (GGA round 1 of the #155 commit, Windows parity in #158):
// creation enforces 0600, but an EXISTING file hardened by nobody else's editor
// (0644 by a bad umask, or a permissive Windows DACL) must not load into the
// digest namespace silently — a readable HMAC key exposes the audit identity
// scheme to every local principal, and a writable one lets them replace the
// namespace. The verdict is platform-specific and lives behind
// auditHashKeyPermissionGate; the load fails closed naming the fix the operator
// runs.
func readAuditHashKeyFile(path string) ([]byte, error) {
	if err := auditHashKeyPermissionGate(path, auditGateFile); err != nil {
		return nil, err
	}
	// #nosec G304 -- path is <operator config dir>/audit-hash-key: a constant
	// filename under a directory the composition root owns (internal/admin.ConfigDir).
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo leer la clave del digest de auditoría",
			Cause:   err,
		}
	}

	key, err := parseAuditHashKeyFile(raw)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// parseAuditHashKeyFile enforces the documented on-disk format: 64 lowercase hex
// characters, tolerant of trailing newlines (an editor-added newline, or a CRLF
// checkout, must not brick the boot). Uppercase is rejected on purpose — the
// format is a contract, and a lenient parser is a second, undocumented one.
func parseAuditHashKeyFile(raw []byte) ([]byte, error) {
	encoded := strings.TrimRight(string(raw), "\r\n")
	if len(encoded) != auditHashKeyHexLen || !isLowerHex(encoded) {
		return nil, &domain.SemanticError{
			Code: domain.ErrCodeInternal,
			Message: fmt.Sprintf(
				"la clave del digest de auditoría es inválida: se esperaban %d caracteres hex en minúsculas",
				auditHashKeyHexLen),
		}
	}

	key, err := hex.DecodeString(encoded)
	if err != nil {
		// Unreachable after the isLowerHex gate; kept so a future relaxation of
		// that gate cannot smuggle a decode failure into a nil key.
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "la clave del digest de auditoría es inválida: no es hex",
			Cause:   err,
		}
	}
	return key, nil
}

// createAuditHashKeyFile generates and persists a fresh random key.
func createAuditHashKeyFile(path string) ([]byte, error) {
	// GGA round 4 finding 1: the create path must gate the EXISTING parent dir
	// BEFORE any key byte is written. The file inherits the parent's ACL at
	// creation (Windows: os.Chmod does not touch ACLs), so gating only after
	// the write left a transient window where an over-permissive inherited
	// DACL already held the secret even though the boot was about to fail.
	// A missing dir falls through: MkdirAll creates it with the tight mode
	// below (file-level enforcement follows through the post-write gate).
	if err := auditHashKeyPermissionGate(filepath.Dir(path), auditGateDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), auditHashKeyDirMode); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el directorio de configuración de la clave del digest de auditoría",
			Cause:   err,
		}
	}
	// GGA round 6 finding 1 (first-boot window): the fresh dir inherits the
	// parent's ACL and os.MkdirAll does NOT establish a restrictive DACL on
	// Windows; re-gate the (now existing) dir BEFORE a single key byte is
	// written so the write-before-evaluate window closes for first boot too.
	if err := auditHashKeyPermissionGate(filepath.Dir(path), auditGateDir); err != nil {
		return nil, err
	}

	key := make([]byte, AuditHashKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo generar la clave del digest de auditoría",
			Cause:   err,
		}
	}

	// #nosec G304 -- same constant filename under the operator config dir as
	// readAuditHashKeyFile; O_EXCL makes the create race explicit below.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, auditHashKeyFileMode)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Lost the create race (server unit + operator TUI booting at
			// once): adopt the existing key instead of forking the namespace.
			return readRacedAuditHashKeyFile(path)
		}
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear la clave del digest de auditoría",
			Cause:   err,
		}
	}

	if _, err := file.WriteString(hex.EncodeToString(key) + "\n"); err != nil {
		_ = file.Close() //nolint:errcheck // the write error is the one worth reporting
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir la clave del digest de auditoría",
			Cause:   err,
		}
	}
	if err := file.Close(); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir la clave del digest de auditoría",
			Cause:   err,
		}
	}
	// Explicit chmod: the mode above only applies at creation, and an inherited
	// loose mode must not survive (same reasoning as admin.WriteCallerID).
	if err := os.Chmod(path, auditHashKeyFileMode); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos de la clave del digest de auditoría",
			Cause:   err,
		}
	}
	// GGA round 1 of the #158 unit (finding 2, Windows parity): the create path
	// is no longer the ungated hole — after the chmod, the SAME platform gate
	// re-evaluates the fresh file + dir so a permissive Windows inherited DACL
	// (os.Chmod does not touch ACLs) or a loose POSIX mode fails the FIRST boot
	// instead of sharing a permissive key until the next read.
	if err := auditHashKeyPermissionGate(path, auditGateFile); err != nil {
		return nil, err
	}
	return key, nil
}

// readRacedAuditHashKeyFile re-reads a key file another process just created.
// The winner may still be mid-write, so a bounded retry avoids failing the boot
// on a zero-byte read; a genuinely malformed file still fails closed once the
// attempts are exhausted.
func readRacedAuditHashKeyFile(path string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < auditHashKeyReadAttempts; attempt++ {
		key, err := readAuditHashKeyFile(path)
		if err == nil {
			return key, nil
		}
		lastErr = err
		time.Sleep(auditHashKeyReadBackoff)
	}
	return nil, lastErr
}

// isLowerHex reports whether s is ASCII lowercase hexadecimal only.
func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

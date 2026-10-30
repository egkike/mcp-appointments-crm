package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// testAuditKeyHex is the fixed test key (32 raw bytes) every pinned assertion
// uses. It is deliberately not random: the pinned digests below are computed
// once from it, so any change to the scheme breaks the test instead of silently
// re-baselining.
const testAuditKeyHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// testAuditCallerID is the caller id of the pinned vector.
const testAuditCallerID = "+5491100000000"

// testAuditDigestPinned is HMAC-SHA-256(key = hex(testAuditKeyHex),
// id = "+5491100000000"), hex-encoded. Computed once, independent of this code
// (python: hmac.new(key, b"+5491100000000", sha256).hexdigest()).
const testAuditDigestPinned = "208d590bd033aefc5eb821081f931a68abc40ca4706854f8ba3b1986bf794043"

// testAuditKey decodes the fixed test key.
func testAuditKey(t *testing.T) []byte {
	t.Helper()
	key, err := hex.DecodeString(testAuditKeyHex)
	if err != nil {
		t.Fatalf("decode test key: %v", err)
	}
	return key
}

// testAuditDigester builds the keyed digester for the fixed test key.
func testAuditDigester(t *testing.T) CallerIDDigester {
	t.Helper()
	digester, err := NewCallerIDDigester(testAuditKey(t))
	if err != nil {
		t.Fatalf("NewCallerIDDigester(test key): %v", err)
	}
	return digester
}

// preserveAuditDigest snapshots the process-wide digest and restores it when the
// test ends. The parallel tests of this package resume only after every
// sequential test (cleanup included) has finished, so a sequential test may
// mutate the global as long as it restores it.
func preserveAuditDigest(t *testing.T) {
	t.Helper()
	auditDigestMu.Lock()
	saved := auditDigest
	auditDigestMu.Unlock()

	t.Cleanup(func() {
		auditDigestMu.Lock()
		auditDigest = saved
		auditDigestMu.Unlock()
	})
}

// clearAuditDigest forces the "not configured" state.
func clearAuditDigest() {
	auditDigestMu.Lock()
	auditDigest = nil
	auditDigestMu.Unlock()
}

// TestNewCallerIDDigester_PinnedVector is the scheme pin: with a fixed key the
// digest of a fixed caller id must equal the hex constant computed once,
// independently of this implementation.
func TestNewCallerIDDigester_PinnedVector(t *testing.T) {
	digester := testAuditDigester(t)

	if got := digester(testAuditCallerID); got != testAuditDigestPinned {
		t.Fatalf("digest(%q) = %q; want the pinned HMAC vector %q", testAuditCallerID, got, testAuditDigestPinned)
	}
}

// TestNewCallerIDDigester_DiffersFromRetiredUnkeyedDigest pins the point of the
// issue: the keyed digest must not reproduce the enumerable unkeyed SHA-256.
func TestNewCallerIDDigester_DiffersFromRetiredUnkeyedDigest(t *testing.T) {
	digester := testAuditDigester(t)

	unkeyed := sha256.Sum256([]byte(testAuditCallerID))
	if got := digester(testAuditCallerID); got == hex.EncodeToString(unkeyed[:]) {
		t.Fatalf("keyed digest equals the retired unkeyed SHA-256; the caller id stays enumerable")
	}
}

// TestNewCallerIDDigester_ShapeAndDeterminism keeps the byte-for-byte shape the
// audit sink validates (64 lowercase hex) and the correlation property readers
// depend on: same id → same digest, different id → different digest.
func TestNewCallerIDDigester_ShapeAndDeterminism(t *testing.T) {
	digester := testAuditDigester(t)

	first := digester(testAuditCallerID)
	if first != digester(testAuditCallerID) {
		t.Error("digest is not deterministic for the same caller id")
	}
	if len(first) != 64 || !isLowerHex(first) {
		t.Errorf("digest = %q; want 64 lowercase hex chars (audit actor_ref shape)", first)
	}
	if other := digester("+5491100000001"); other == first {
		t.Error("different caller ids produced the same digest")
	}
}

// TestNewCallerIDDigester_RejectsWrongKeyLength fails closed instead of padding a
// short key (a short key weakens the MAC silently).
func TestNewCallerIDDigester_RejectsWrongKeyLength(t *testing.T) {
	for _, keyLen := range []int{0, 1, AuditHashKeySize - 1, AuditHashKeySize + 1} {
		digester, err := NewCallerIDDigester(make([]byte, keyLen))
		if err == nil {
			t.Errorf("NewCallerIDDigester(%d-byte key) error = nil, want a rejection", keyLen)
		}
		if digester != nil {
			t.Errorf("NewCallerIDDigester(%d-byte key) returned a digester alongside the error", keyLen)
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Errorf("NewCallerIDDigester(%d-byte key) error = %T (%v), want *domain.SemanticError", keyLen, err, err)
		}
	}
}

// TestNewCallerIDDigester_CopiesKey guards the namespace: mutating the caller's
// key slice after construction must not change the installed digest.
func TestNewCallerIDDigester_CopiesKey(t *testing.T) {
	key := testAuditKey(t)
	digester, err := NewCallerIDDigester(key)
	if err != nil {
		t.Fatalf("NewCallerIDDigester: %v", err)
	}
	want := digester(testAuditCallerID)

	for i := range key {
		key[i] = 0xff
	}
	if got := digester(testAuditCallerID); got != want {
		t.Errorf("digest changed after the caller mutated its key slice: got %q, want %q", got, want)
	}
}

// TestLoadOrCreateAuditHashKey_CreatesKeyFileOnFirstUse pins the on-disk
// contract: 0700 dir, 0600 file, 64 lowercase hex chars plus a trailing newline,
// 32 raw bytes once decoded.
func TestLoadOrCreateAuditHashKey_CreatesKeyFileOnFirstUse(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")

	key, err := LoadOrCreateAuditHashKey(configDir)
	if err != nil {
		t.Fatalf("LoadOrCreateAuditHashKey: %v", err)
	}
	if len(key) != AuditHashKeySize {
		t.Fatalf("key length = %d bytes, want %d", len(key), AuditHashKeySize)
	}

	keyPath := filepath.Join(configDir, AuditHashKeyFileName)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("key file mode = %04o, want 0600", got)
	}

	dirInfo, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("config dir mode = %04o, want 0700", got)
	}

	// #nosec G304 -- keyPath is <throwaway t.TempDir()>/config/audit-hash-key.
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	encoded := strings.TrimRight(string(raw), "\r\n")
	if len(encoded) != 64 || !isLowerHex(encoded) {
		t.Errorf("key file content = %q; want 64 lowercase hex chars", encoded)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Errorf("key file content = %q; want a trailing newline", raw)
	}
	if hex.EncodeToString(key) != encoded {
		t.Error("returned key does not match the persisted key")
	}
}

// TestLoadOrCreateAuditHashKey_SecondCallReturnsSameKey pins the namespace: the
// key is created ONCE and reused, never regenerated (a regeneration would make
// every historical digest unverifiable).
func TestLoadOrCreateAuditHashKey_SecondCallReturnsSameKey(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")

	first, err := LoadOrCreateAuditHashKey(configDir)
	if err != nil {
		t.Fatalf("first LoadOrCreateAuditHashKey: %v", err)
	}
	second, err := LoadOrCreateAuditHashKey(configDir)
	if err != nil {
		t.Fatalf("second LoadOrCreateAuditHashKey: %v", err)
	}
	if hex.EncodeToString(first) != hex.EncodeToString(second) {
		t.Fatalf("second load returned a different key: %x != %x", first, second)
	}
}

// TestLoadOrCreateAuditHashKey_ToleratesTrailingNewlines accepts the editor- and
// checkout-friendly variants of the same 64 hex chars.
func TestLoadOrCreateAuditHashKey_ToleratesTrailingNewlines(t *testing.T) {
	for name, contents := range map[string]string{
		"no newline":     testAuditKeyHex,
		"one newline":    testAuditKeyHex + "\n",
		"two newlines":   testAuditKeyHex + "\n\n",
		"crlf newline":   testAuditKeyHex + "\r\n",
		"crlf and extra": testAuditKeyHex + "\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			configDir := tDir700(t)
			writeKeyFile(t, configDir, contents)

			key, err := LoadOrCreateAuditHashKey(configDir)
			if err != nil {
				t.Fatalf("LoadOrCreateAuditHashKey: %v", err)
			}
			if hex.EncodeToString(key) != testAuditKeyHex {
				t.Errorf("loaded key = %x; want the file's key", key)
			}
		})
	}
}

// TestLoadOrCreateAuditHashKey_RejectsMalformedKey fails closed on any key that
// is not exactly 64 lowercase hex chars: no trimming of extra bytes, no
// uppercase leniency, no fallback key.
func TestLoadOrCreateAuditHashKey_RejectsMalformedKey(t *testing.T) {
	for name, contents := range map[string]string{
		"empty":            "",
		"only newline":     "\n",
		"too short":        testAuditKeyHex[:62],
		"too long":         testAuditKeyHex + "ab",
		"uppercase":        strings.ToUpper(testAuditKeyHex),
		"non-hex":          strings.Repeat("z", 64),
		"raw bytes":        "\x00\x01\x02\x03",
		"padded with text": testAuditKeyHex + " not-a-key",
	} {
		t.Run(name, func(t *testing.T) {
			configDir := tDir700(t)
			writeKeyFile(t, configDir, contents)

			key, err := LoadOrCreateAuditHashKey(configDir)
			if err == nil {
				t.Fatalf("LoadOrCreateAuditHashKey accepted a malformed key: %x", key)
			}
			if key != nil {
				t.Errorf("LoadOrCreateAuditHashKey returned a key alongside the error: %x", key)
			}
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Errorf("error = %T (%v), want *domain.SemanticError", err, err)
			}
		})
	}
}

// TestLoadOrCreateAuditHashKey_RejectsEmptyConfigDir refuses to guess a location:
// without a config dir the key must not land anywhere.
func TestLoadOrCreateAuditHashKey_RejectsEmptyConfigDir(t *testing.T) {
	for _, configDir := range []string{"", "   "} {
		key, err := LoadOrCreateAuditHashKey(configDir)
		if err == nil {
			t.Fatalf("LoadOrCreateAuditHashKey(%q) error = nil, want a fail-closed rejection", configDir)
		}
		if key != nil {
			t.Errorf("LoadOrCreateAuditHashKey(%q) returned a key alongside the error", configDir)
		}
	}
}

// TestLoadOrCreateAuditHashKey_FailsClosedWhenDirectoryCannotBeCreated is the
// unwritable-installation case: no key, no fallback, a semantic error.
func TestLoadOrCreateAuditHashKey_FailsClosedWhenDirectoryCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	key, err := LoadOrCreateAuditHashKey(filepath.Join(blocker, "config"))
	if err == nil {
		t.Fatalf("LoadOrCreateAuditHashKey under a file parent error = nil, want a fail-closed rejection")
	}
	if key != nil {
		t.Errorf("LoadOrCreateAuditHashKey returned a key alongside the error: %x", key)
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Errorf("error = %T (%v), want *domain.SemanticError", err, err)
	}
}

// TestCreateAuditHashKeyFile_AdoptsExistingKey covers the create race (server
// unit + operator TUI booting at once): the loser reads the winner's key instead
// of forking the digest namespace.
func TestCreateAuditHashKeyFile_AdoptsExistingKey(t *testing.T) {
	configDir := tDir700(t)
	writeKeyFile(t, configDir, testAuditKeyHex)
	keyPath := filepath.Join(configDir, AuditHashKeyFileName)

	key, err := createAuditHashKeyFile(keyPath)
	if err != nil {
		t.Fatalf("createAuditHashKeyFile on an existing key: %v", err)
	}
	if hex.EncodeToString(key) != testAuditKeyHex {
		t.Errorf("adopted key = %x; want the existing key", key)
	}

	// #nosec G304 -- keyPath is <throwaway t.TempDir()>/audit-hash-key.
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	if strings.TrimRight(string(raw), "\r\n") != testAuditKeyHex {
		t.Errorf("key file was rewritten: %q", raw)
	}
}

// TestCreateAuditHashKeyFile_RejectsMalformedExistingKey: losing the race to a
// corrupt file must not accept it (the retry window expires and the boot fails).
func TestCreateAuditHashKeyFile_RejectsMalformedExistingKey(t *testing.T) {
	configDir := tDir700(t)
	writeKeyFile(t, configDir, "short")
	keyPath := filepath.Join(configDir, AuditHashKeyFileName)

	key, err := createAuditHashKeyFile(keyPath)
	if err == nil {
		t.Fatalf("createAuditHashKeyFile on a malformed existing key error = nil, want a rejection")
	}
	if key != nil {
		t.Errorf("createAuditHashKeyFile returned a key alongside the error: %x", key)
	}
}

// TestConfigureAuditDigest_InstallsKeyedDigest pins the boot seam: configuring
// the loaded key makes HashCallerID produce the keyed digest.
func TestConfigureAuditDigest_InstallsKeyedDigest(t *testing.T) {
	preserveAuditDigest(t)

	if err := ConfigureAuditDigest(testAuditKey(t)); err != nil {
		t.Fatalf("ConfigureAuditDigest: %v", err)
	}
	if !AuditDigestConfigured() {
		t.Fatal("AuditDigestConfigured() = false right after a successful ConfigureAuditDigest")
	}
	if got := HashCallerID(testAuditCallerID); got != testAuditDigestPinned {
		t.Errorf("HashCallerID(%q) = %q; want the pinned keyed vector %q", testAuditCallerID, got, testAuditDigestPinned)
	}
}

// TestConfigureAuditDigest_RejectsMalformedKeyWithoutClobbering keeps a bad
// reconfiguration from disarming the digest already in place.
func TestConfigureAuditDigest_RejectsMalformedKeyWithoutClobbering(t *testing.T) {
	preserveAuditDigest(t)

	if err := ConfigureAuditDigest(testAuditKey(t)); err != nil {
		t.Fatalf("ConfigureAuditDigest: %v", err)
	}
	if err := ConfigureAuditDigest([]byte("too-short")); err == nil {
		t.Fatal("ConfigureAuditDigest with a short key error = nil, want a rejection")
	}
	if got := HashCallerID(testAuditCallerID); got != testAuditDigestPinned {
		t.Errorf("HashCallerID after a rejected reconfiguration = %q; want the previous keyed vector", got)
	}
}

// TestHashCallerID_UnconfiguredFailsClosed is the fail-closed contract: with no
// digest configured the reference is empty — never the retired unkeyed SHA-256
// (which is exactly the enumerable value ADR-0021 removes).
func TestHashCallerID_UnconfiguredFailsClosed(t *testing.T) {
	preserveAuditDigest(t)
	clearAuditDigest()

	if AuditDigestConfigured() {
		t.Fatal("AuditDigestConfigured() = true after clearing the digest")
	}
	if got := HashCallerID(testAuditCallerID); got != "" {
		t.Fatalf("HashCallerID unconfigured = %q; want the empty fail-closed reference", got)
	}

	unkeyed := sha256.Sum256([]byte(testAuditCallerID))
	if HashCallerID(testAuditCallerID) == hex.EncodeToString(unkeyed[:]) {
		t.Fatal("HashCallerID fell back to the retired unkeyed SHA-256")
	}
}

// TestHashCallerID_FullConfigRoundTrip walks the whole production seam: create
// the key file in a throwaway config dir, load it, configure it, digest.
func TestHashCallerID_FullConfigRoundTrip(t *testing.T) {
	preserveAuditDigest(t)

	configDir := filepath.Join(t.TempDir(), "config")
	key, err := LoadOrCreateAuditHashKey(configDir)
	if err != nil {
		t.Fatalf("LoadOrCreateAuditHashKey: %v", err)
	}
	if err := ConfigureAuditDigest(key); err != nil {
		t.Fatalf("ConfigureAuditDigest: %v", err)
	}

	if got := HashCallerID(testAuditCallerID); got == "" {
		t.Fatal("HashCallerID = \"\" after a full load+configure round trip")
	}
	if got := HashCallerID(testAuditCallerID); len(got) != 64 || !isLowerHex(got) {
		t.Errorf("HashCallerID = %q; want 64 lowercase hex chars", got)
	}
}

// writeKeyFile seeds <dir>/audit-hash-key with contents, creating dir.
func writeKeyFile(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	// #nosec G304 -- dir is a throwaway t.TempDir(); the filename is the constant
	// AuditHashKeyFileName.
	if err := os.WriteFile(filepath.Join(dir, AuditHashKeyFileName), []byte(contents), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
}

// TestLoadOrCreateAuditHashKey_RejectsWorldReadableKeyFile pins the
// permissions gate on the READ path (GGA round 1 of the #155 commit): a key
// file created loose (e.g. 0644 by an editor or a bad umask) must fail closed
// with the exact fix in the message, never load into the digest namespace.
func TestLoadOrCreateAuditHashKey_RejectsWorldReadableKeyFile(t *testing.T) {
	dir := tDir700(t)
	hexKey := strings.Repeat("ab", 32)
	keyPath := filepath.Join(dir, AuditHashKeyFileName)
	// Deliberately world-readable: the loose mode is the fixture under test
	// (#nosec G306 -- the gate under test rejects it before any use).
	if err := os.WriteFile(keyPath, []byte(hexKey+"\n"), 0o644); err != nil { //nolint:gosec // fixture: the loose mode IS the point
		t.Fatalf("seed loose key file: %v", err)
	}

	_, err := LoadOrCreateAuditHashKey(dir)
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("err = %v, want a *domain.SemanticError for a loose key file", err)
	}
	if !strings.Contains(semErr.Message, "0600") {
		t.Errorf("message = %q, want it to name the chmod fix", semErr.Message)
	}
}

// TestLoadOrCreateAuditHashKey_RejectsGroupWritableConfigDir pins the directory
// gate: an existing config dir that lets group/others WRITE would let anyone
// replace the digest namespace - reject the load naming the chmod fix.
func TestLoadOrCreateAuditHashKey_RejectsGroupWritableConfigDir(t *testing.T) {
	dir := tDir700(t)
	if err := os.WriteFile(filepath.Join(dir, AuditHashKeyFileName), []byte(strings.Repeat("cd", 32)+"\n"), 0o600); err != nil {
		t.Fatalf("seed key file: %v", err)
	}
	// Deliberately group-writable: a group member could delete-and-rewrite the
	// key file through the directory despite its 0600 mode (#nosec G302 --
	// the gate under test rejects it before any use).
	if err := os.Chmod(dir, 0o770); err != nil { //nolint:gosec // fixture: the loose dir IS the point
		t.Fatalf("loosen config dir: %v", err)
	}

	_, err := LoadOrCreateAuditHashKey(dir)
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("err = %v, want a *domain.SemanticError for a loose config dir", err)
	}
	if !strings.Contains(semErr.Message, "0700") {
		t.Errorf("message = %q, want it to name the chmod fix", semErr.Message)
	}
}

// preserveAuditKeyGate snapshots the platform permission gate and restores it
// when the test ends (same reasoning as preserveAuditDigest: parallel tests in
// this package resume only after every sequential test has finished).
func preserveAuditKeyGate(t *testing.T) {
	t.Helper()
	saved := auditHashKeyPermissionGate
	t.Cleanup(func() { auditHashKeyPermissionGate = saved })
}

// TestReadAuditHashKeyFile_RoutesEveryReadThroughThePermissionGate pins the
// #158 reroute: the platform access-control verdict comes from the gate and
// from nowhere else. A 0644 key file — which the POSIX gate rejects — is
// ACCEPTED when the gate answers "safe", which is only possible if no inline
// mode-bit check survives on the read path.
func TestReadAuditHashKeyFile_RoutesEveryReadThroughThePermissionGate(t *testing.T) {
	dir := tDir700(t)
	keyPath := filepath.Join(dir, AuditHashKeyFileName)
	// Deliberately loose mode: with the checks behind the gate only the stubbed
	// verdict decides (#nosec G306 -- the loose mode is the assertion's premise).
	if err := os.WriteFile(keyPath, []byte(testAuditKeyHex+"\n"), 0o644); err != nil { //nolint:gosec // fixture: the loose mode IS the point
		t.Fatalf("seed loose key file: %v", err)
	}

	preserveAuditKeyGate(t)
	var gatedPaths []string
	var gatedKinds []auditGateTarget
	auditHashKeyPermissionGate = func(path string, kind auditGateTarget) error {
		gatedPaths = append(gatedPaths, path)
		gatedKinds = append(gatedKinds, kind)
		return nil
	}

	key, err := readAuditHashKeyFile(keyPath)
	if err != nil {
		t.Fatalf("readAuditHashKeyFile with a permissive gate: %v", err)
	}
	if hex.EncodeToString(key) != testAuditKeyHex {
		t.Errorf("key = %x; want the seeded key", key)
	}
	if len(gatedPaths) != 1 || gatedPaths[0] != keyPath {
		t.Fatalf("gate paths = %q; want exactly one call with the key path %q", gatedPaths, keyPath)
	}
	if len(gatedKinds) != 1 || gatedKinds[0] != auditGateFile {
		t.Fatalf("gate kinds = %v; want auditGateFile", gatedKinds)
	}
}

// TestReadAuditHashKeyFile_PropagatesGateRejection pins fail-closed semantics:
// a rejecting gate is a fatal load, returned verbatim with no key alongside it
// (the #158 bug pattern was a gate that never produced a verdict at all).
func TestReadAuditHashKeyFile_PropagatesGateRejection(t *testing.T) {
	dir := tDir700(t)
	writeKeyFile(t, dir, testAuditKeyHex+"\n")
	keyPath := filepath.Join(dir, AuditHashKeyFileName)

	rejection := &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: "gate sentinel: el archivo concede lectura a otro principal",
		Cause:   os.ErrPermission,
	}
	preserveAuditKeyGate(t)
	auditHashKeyPermissionGate = func(string, auditGateTarget) error { return rejection }

	key, err := readAuditHashKeyFile(keyPath)
	if !errors.Is(err, rejection) {
		t.Fatalf("err = %v, want the gate's rejection returned verbatim", err)
	}
	if key != nil {
		t.Errorf("readAuditHashKeyFile returned a key alongside the gate rejection: %x", key)
	}
}

// tDir700 is the normal-case keyfile fixture directory: it compensates this
// environment's t.TempDir() group-writable layout (umask 0002) so the tests
// exercise the gate only where they intend to.
func tDir700(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// #nosec G302 -- compensating this environment's group-writable t.TempDir
	// layout (umask 0002); 0700 is the fixture's documented mode, not looser.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("tighten temp config dir to 0700: %v", err)
	}
	return dir
}

// TestReadAuditHashKeyFile_RejectsSymlinkKey pins the symlink rule (GGA round
// 2 of the #158 unit): the POSIX gate reads the TARGET when following a
// symlink, so a swapped key can point anywhere; the gate rejects a symlinked
// key or directory instead of ever trusting the hop.
func TestReadAuditHashKeyFile_RejectsSymlinkKey(t *testing.T) {
	dir := tDir700(t)
	realPath := filepath.Join(dir, "real-key")
	keyHex := strings.Repeat("ef", 32)
	if err := os.WriteFile(realPath, []byte(keyHex+"\n"), 0o600); err != nil {
		t.Fatalf("seed real key: %v", err)
	}
	linkPath := filepath.Join(dir, AuditHashKeyFileName)
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	_, err := readAuditHashKeyFile(linkPath)
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("readAuditHashKeyFile(symlink) err = %v, want a *domain.SemanticError", err)
	}
	if !strings.Contains(semErr.Message, "symlink") {
		t.Errorf("message = %q, want it to name the symlink rule", semErr.Message)
	}
}

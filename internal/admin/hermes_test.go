package admin

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"gopkg.in/yaml.v3"
)

const (
	testHermesURL   = "http://127.0.0.1:3000/mcp"
	testHermesPhone = "+5491100000000"
)

// readHermesFile reads a file the test itself created under t.TempDir().
func readHermesFile(t *testing.T, path string) []byte {
	t.Helper()
	// #nosec G304 -- path is inside t.TempDir() created by the test.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// hermesConfigInTempDir returns a path under a fresh temp dir and seeds no
// file, mirroring a new install where ~/.hermes does not exist yet.
func hermesConfigInTempDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), hermesDirName, HermesConfigFileName)
}

func TestHermesConfigPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME override does not drive os.UserHomeDir on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := HermesConfigPath()
	if err != nil {
		t.Fatalf("HermesConfigPath() error = %v", err)
	}
	want := filepath.Join(home, hermesDirName, HermesConfigFileName)
	if got != want {
		t.Errorf("HermesConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadHermesConfig_AbsentFileReturnsEmptyDocument(t *testing.T) {
	path := hermesConfigInTempDir(t)

	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() error = %v, want nil for an absent file", err)
	}
	if doc == nil {
		t.Fatal("LoadHermesConfig() = nil, want a non-nil empty document")
	}
	if len(doc) != 0 {
		t.Errorf("LoadHermesConfig() = %v, want an empty document", doc)
	}
}

func TestLoadHermesConfig_EmptyFileReturnsEmptyDocument(t *testing.T) {
	path := hermesConfigInTempDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("seed empty file: %v", err)
	}

	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() error = %v", err)
	}
	if len(doc) != 0 {
		t.Errorf("LoadHermesConfig() = %v, want an empty document", doc)
	}
}

func TestLoadHermesConfig_ParsesExistingMapping(t *testing.T) {
	path := hermesConfigInTempDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seed := []byte("model: hermes-default\nmcp_servers:\n  openai:\n    url: https://api.example/mcp\n")
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() error = %v", err)
	}
	if got := doc["model"]; got != "hermes-default" {
		t.Errorf("doc[model] = %v, want %q", got, "hermes-default")
	}
	servers, ok := doc[hermesServersSection].(map[string]any)
	if !ok {
		t.Fatalf("doc[mcp_servers] = %T, want map[string]any", doc[hermesServersSection])
	}
	if _, ok := servers["openai"]; !ok {
		t.Errorf("doc[mcp_servers] lost the openai server: %v", servers)
	}
}

func TestLoadHermesConfig_InvalidYAMLIsSemanticErrorWithoutPath(t *testing.T) {
	path := hermesConfigInTempDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("mcp_servers: [unbalanced\n"), 0o600); err != nil {
		t.Fatalf("seed malformed config: %v", err)
	}

	_, err := LoadHermesConfig(path)
	if err == nil {
		t.Fatal("LoadHermesConfig() error = nil, want a semantic failure")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("LoadHermesConfig() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("LoadHermesConfig() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
	if strings.Contains(semErr.Message, path) {
		t.Errorf("LoadHermesConfig() message = %q, want no filesystem path", semErr.Message)
	}
}

func TestSetHermesServer_AbsentFileFlowCreatesEntry(t *testing.T) {
	path := hermesConfigInTempDir(t)

	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() error = %v", err)
	}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("WriteHermesConfig() error = %v", err)
	}

	reloaded, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	entry := hermesEntryFrom(t, reloaded)
	if entry[hermesURLKey] != testHermesURL {
		t.Errorf("entry url = %v, want %q", entry[hermesURLKey], testHermesURL)
	}
}

// hermesEntryFrom extracts the mcp_servers.<HermesServerName> mapping from a
// loaded document, failing the test when the shape is wrong.
func hermesEntryFrom(t *testing.T, doc HermesDocument) map[string]any {
	t.Helper()
	servers, ok := doc[hermesServersSection].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers = %T, want map[string]any", doc[hermesServersSection])
	}
	entry, ok := servers[HermesServerName].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers.%s = %T, want map[string]any", HermesServerName, servers[HermesServerName])
	}
	return entry
}

func TestSetHermesServer_PreservesUnknownKeysAndOtherServers(t *testing.T) {
	path := hermesConfigInTempDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seed := strings.Join([]string{
		"model: hermes-default",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"    headers:",
		"      Authorization: Bearer token",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:9999/old",
		"    headers:",
		"      X-Caller-Id: \"+5490000000000\"",
		"      X-Stale: should-be-dropped",
		"custom_section:",
		"  nested: true",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() error = %v", err)
	}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}

	if got := doc["model"]; got != "hermes-default" {
		t.Errorf("unknown top-level key lost: doc[model] = %v", got)
	}
	custom, ok := doc["custom_section"].(map[string]any)
	if !ok || custom["nested"] != true {
		t.Errorf("unknown nested key lost: doc[custom_section] = %v", doc["custom_section"])
	}

	servers, ok := doc[hermesServersSection].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers = %T, want map[string]any", doc[hermesServersSection])
	}
	openai, ok := servers["openai"].(map[string]any)
	if !ok {
		t.Fatalf("other server lost: mcp_servers[openai] = %v", servers["openai"])
	}
	if openai[hermesURLKey] != "https://api.example/mcp" {
		t.Errorf("other server url mutated: %v", openai[hermesURLKey])
	}

	replaced, ok := servers[HermesServerName].(map[string]any)
	if !ok {
		t.Fatalf("entry = %T, want map[string]any", servers[HermesServerName])
	}
	if replaced[hermesURLKey] != testHermesURL {
		t.Errorf("entry url = %v, want %q", replaced[hermesURLKey], testHermesURL)
	}
	headers, ok := replaced[hermesHeadersKey].(map[string]any)
	if !ok {
		t.Fatalf("entry headers = %T, want map[string]any", replaced[hermesHeadersKey])
	}
	if _, stale := headers["X-Stale"]; stale {
		t.Errorf("previous entry keys survived the replace: %v", headers)
	}
}

func TestSetHermesServer_IdempotentReMerge(t *testing.T) {
	path := hermesConfigInTempDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A real Hermes config with an unrelated server and an unrelated top-level
	// key so the idempotency proof spans the full document, not just the entry.
	seed := "other: keep\nmcp_servers:\n  openai:\n    url: https://api.example/mcp\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	writeOnce := func() []byte {
		t.Helper()
		doc, err := LoadHermesConfig(path)
		if err != nil {
			t.Fatalf("LoadHermesConfig() error = %v", err)
		}
		if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
			t.Fatalf("SetHermesServer() error = %v", err)
		}
		if err := WriteHermesConfig(path, doc); err != nil {
			t.Fatalf("WriteHermesConfig() error = %v", err)
		}
		return readHermesFile(t, path)
	}

	first := writeOnce()
	second := writeOnce()
	if !bytes.Equal(first, second) {
		t.Errorf("re-running the merge changed the file:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	reloaded, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	servers, ok := reloaded[hermesServersSection].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers is not a mapping after re-merge: %#v", reloaded[hermesServersSection])
	}
	if _, ok := servers["openai"]; !ok {
		t.Errorf("idempotent re-merge dropped the other server: %v", servers)
	}
	if len(servers) != 2 {
		t.Errorf("mcp_servers has %d entries, want 2 (openai + mcp-appointments)", len(servers))
	}
}

func TestSetHermesServer_NormalizesSurroundingWhitespaceInURL(t *testing.T) {
	doc := HermesDocument{}
	if err := doc.SetHermesServer("  "+testHermesURL+"  ", testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	entry := hermesEntryFrom(t, doc)
	if entry[hermesURLKey] != testHermesURL {
		t.Errorf("entry url = %q, want the trimmed %q", entry[hermesURLKey], testHermesURL)
	}
}

func TestSetHermesServer_RejectsNonMappingSection(t *testing.T) {
	doc := HermesDocument{hermesServersSection: "not-a-map"}

	err := doc.SetHermesServer(testHermesURL, testHermesPhone)
	if err == nil {
		t.Fatal("SetHermesServer() error = nil, want a semantic failure")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("SetHermesServer() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("SetHermesServer() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}

func TestSetHermesServer_ReusesPhoneValidation(t *testing.T) {
	doc := HermesDocument{}

	gotErr := doc.SetHermesServer(testHermesURL, "not-a-phone")
	wantErr := ValidatePhone("not-a-phone")
	if gotErr == nil || wantErr == nil {
		t.Fatalf("expected both SetHermesServer and ValidatePhone to reject the phone")
	}
	if gotErr.Error() != wantErr.Error() {
		t.Errorf("SetHermesServer() phone error = %q, want the ValidatePhone message %q", gotErr.Error(), wantErr.Error())
	}

	var semErr *domain.SemanticError
	if !errors.As(gotErr, &semErr) {
		t.Fatalf("SetHermesServer() phone error = %T, want *domain.SemanticError", gotErr)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("SetHermesServer() phone code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}

func TestSetHermesServer_RejectsInvalidURL(t *testing.T) {
	doc := HermesDocument{}

	err := doc.SetHermesServer("ftp://example.com/mcp", testHermesPhone)
	if err == nil {
		t.Fatal("SetHermesServer() error = nil, want a URL validation failure")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("SetHermesServer() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("SetHermesServer() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}

func TestValidateHermesURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "http loopback", rawURL: "http://127.0.0.1:3000/mcp"},
		{name: "https host", rawURL: "https://example.com/mcp"},
		{name: "http host without path", rawURL: "http://example.com:8080"},
		{name: "empty", rawURL: "", wantErr: true},
		{name: "missing scheme", rawURL: "127.0.0.1:3000/mcp", wantErr: true},
		{name: "unsupported scheme", rawURL: "ftp://example.com", wantErr: true},
		{name: "missing host", rawURL: "http:///mcp", wantErr: true},
		{name: "garbage", rawURL: "http://%zz", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHermesURL(tc.rawURL)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateHermesURL(%q) = nil, want an error", tc.rawURL)
				}
				var semErr *domain.SemanticError
				if !errors.As(err, &semErr) {
					t.Fatalf("ValidateHermesURL(%q) = %T, want *domain.SemanticError", tc.rawURL, err)
				}
				if semErr.Code != domain.ErrCodeInvalidInput {
					t.Errorf("ValidateHermesURL(%q) code = %q, want %q", tc.rawURL, semErr.Code, domain.ErrCodeInvalidInput)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateHermesURL(%q) error = %v, want nil", tc.rawURL, err)
			}
		})
	}
}

func TestHermesEndpointURL(t *testing.T) {
	tests := []struct {
		name    string
		bind    string
		port    string
		want    string
		wantErr bool
	}{
		{name: "ipv4", bind: "127.0.0.1", port: "3000", want: "http://127.0.0.1:3000/mcp"},
		{name: "ipv6 is bracketed", bind: "::1", port: "3000", want: "http://[::1]:3000/mcp"},
		{name: "missing bind", bind: "", port: "3000", wantErr: true},
		{name: "missing port", bind: "127.0.0.1", port: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := HermesEndpointURL(tc.bind, tc.port)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("HermesEndpointURL(%q, %q) = %q, want an error", tc.bind, tc.port, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("HermesEndpointURL(%q, %q) error = %v", tc.bind, tc.port, err)
			}
			if got != tc.want {
				t.Errorf("HermesEndpointURL(%q, %q) = %q, want %q", tc.bind, tc.port, got, tc.want)
			}
		})
	}
}

func TestRenderHermesSnippet_MatchesWriterBlock(t *testing.T) {
	path := hermesConfigInTempDir(t)
	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("LoadHermesConfig() error = %v", err)
	}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("WriteHermesConfig() error = %v", err)
	}
	written := readHermesFile(t, path)

	snippet, err := RenderHermesSnippet(testHermesURL, testHermesPhone)
	if err != nil {
		t.Fatalf("RenderHermesSnippet() error = %v", err)
	}
	if snippet != string(written) {
		t.Errorf("snippet drifted from the writer block:\nsnippet:\n%s\nwritten:\n%s", snippet, written)
	}
}

func TestRenderHermesSnippet_ExactQuotedBlock(t *testing.T) {
	snippet, err := RenderHermesSnippet(testHermesURL, testHermesPhone)
	if err != nil {
		t.Fatalf("RenderHermesSnippet() error = %v", err)
	}

	want := strings.Join([]string{
		"mcp_servers:",
		"  mcp-appointments:",
		"    headers:",
		`      X-Caller-Id: "+5491100000000"`,
		"    url: http://127.0.0.1:3000/mcp",
		"",
	}, "\n")
	if snippet != want {
		t.Errorf("RenderHermesSnippet() =\n%s\nwant:\n%s", snippet, want)
	}
}

func TestRenderHermesSnippet_RejectsInvalidInputs(t *testing.T) {
	if _, err := RenderHermesSnippet("ftp://example.com", testHermesPhone); err == nil {
		t.Error("RenderHermesSnippet() with an invalid URL = nil, want an error")
	}
	if _, err := RenderHermesSnippet(testHermesURL, "not-a-phone"); err == nil {
		t.Error("RenderHermesSnippet() with an invalid phone = nil, want an error")
	}
}

func TestWriteHermesConfig_QuotedPhoneEmissionContract(t *testing.T) {
	path := hermesConfigInTempDir(t)
	doc := HermesDocument{}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("WriteHermesConfig() error = %v", err)
	}
	raw := readHermesFile(t, path)

	// (b) raw emitted bytes carry the double-quoted form.
	quoted := `X-Caller-Id: "` + testHermesPhone + `"`
	if !strings.Contains(string(raw), quoted) {
		t.Errorf("raw config does not contain %q:\n%s", quoted, raw)
	}
	// The unquoted form must not appear anywhere; it would parse as an integer.
	unquoted := "X-Caller-Id: " + testHermesPhone
	if strings.Contains(string(raw), unquoted) {
		t.Errorf("raw config contains the unquoted form %q:\n%s", unquoted, raw)
	}

	// (a) round-trip decode returns the exact string value.
	var decoded map[string]any
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal round-trip: %v", err)
	}
	servers := decoded[hermesServersSection].(map[string]any)
	entry := servers[HermesServerName].(map[string]any)
	headers := entry[hermesHeadersKey].(map[string]any)
	got, ok := headers[HermesCallerIDHeader].(string)
	if !ok {
		t.Fatalf("%s decoded to %T, want string", HermesCallerIDHeader, headers[HermesCallerIDHeader])
	}
	if got != testHermesPhone {
		t.Errorf("%s = %q, want %q", HermesCallerIDHeader, got, testHermesPhone)
	}
}

func TestWriteHermesConfig_CreatesRestrictedDirAndFile(t *testing.T) {
	path := hermesConfigInTempDir(t)
	dir := filepath.Dir(path)
	doc := HermesDocument{}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("WriteHermesConfig() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != hermesDirMode {
		t.Errorf("created dir mode = %04o, want %04o", got, hermesDirMode)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != hermesFileMode {
		t.Errorf("created file mode = %04o, want %04o", got, hermesFileMode)
	}
}

func TestWriteHermesConfig_DoesNotChmodPreexistingDir(t *testing.T) {
	path := hermesConfigInTempDir(t)
	dir := filepath.Dir(path)
	// #nosec G301 -- the fixture must seed a pre-existing permissive dir (0755);
	// a tighter mode would make the no-chmod assertion vacuous.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Seed a Hermes-owned file so the write takes the merge/replace path.
	// #nosec G306 -- the fixture must seed a pre-existing permissive file (0644).
	if err := os.WriteFile(path, []byte("model: hermes-default\n"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	doc := HermesDocument{}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("WriteHermesConfig() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o755 {
		t.Errorf("pre-existing dir mode = %04o, want the untouched 0755", got)
	}

	// The temp file is the one we own, so the replaced config is 0600.
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != hermesFileMode {
		t.Errorf("config file mode = %04o, want %04o", got, hermesFileMode)
	}
}

func TestWriteHermesConfig_LeavesNoTemporaryFileBehind(t *testing.T) {
	path := hermesConfigInTempDir(t)
	dir := filepath.Dir(path)
	doc := HermesDocument{}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("WriteHermesConfig() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestWriteHermesConfig_UnwritableTargetIsSemanticErrorWithoutPath(t *testing.T) {
	// A regular file cannot be a parent directory, so ensureHermesDir fails
	// before any temp file is created.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	path := filepath.Join(blocker, HermesConfigFileName)

	doc := HermesDocument{}
	if err := doc.SetHermesServer(testHermesURL, testHermesPhone); err != nil {
		t.Fatalf("SetHermesServer() error = %v", err)
	}

	err := WriteHermesConfig(path, doc)
	if err == nil {
		t.Fatal("WriteHermesConfig() error = nil, want a semantic failure")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("WriteHermesConfig() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInternal {
		t.Errorf("WriteHermesConfig() code = %q, want %q", semErr.Code, domain.ErrCodeInternal)
	}
	if strings.Contains(semErr.Message, path) || strings.Contains(semErr.Message, blocker) {
		t.Errorf("WriteHermesConfig() message = %q, want no filesystem path", semErr.Message)
	}
}

func TestWriteHermesConfig_EmptyPathIsRejected(t *testing.T) {
	err := WriteHermesConfig("", HermesDocument{})
	if err == nil {
		t.Fatal("WriteHermesConfig(\"\") error = nil, want a validation error")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("WriteHermesConfig(\"\") error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("WriteHermesConfig(\"\") code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}

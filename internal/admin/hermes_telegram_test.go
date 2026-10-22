package admin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"gopkg.in/yaml.v3"
)

// Telegram owner-bot fixtures (feat-whatsapp-bot Phase 5, D1): the private
// Telegram bot the OWNER uses outside the business channel (ADR-0018 Decision
// 3b). The three values live in the mcp_servers.mcp-appointments env block.
const (
	validTelegramToken  = "123456789:short-fake-token"
	validTelegramChatID = "987654321"
	validOwnerPhone     = "+5491100000001"
)

// seedTelegramFixture loads a Hermes document with the server entry set (url +
// quoted X-Caller-Id) plus a foreign top-level key and a foreign sibling
// server, so the merge must preserve all of them.
func seedTelegramFixture(t *testing.T) HermesDocument {
	t.Helper()
	doc, err := LoadHermesConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("load empty document: %v", err)
	}
	if err := doc.SetHermesServer("http://127.0.0.1:3000/mcp", validOwnerPhone); err != nil {
		t.Fatalf("seed server entry: %v", err)
	}
	root := doc.storage().root
	telemetry := &yaml.Node{Kind: yaml.MappingNode}
	putChild(telemetry, "enabled", plainScalar("true"))
	putChild(root, "telemetry", telemetry)
	servers, _ := childMapping(root, hermesServersSection)
	otherEntry := &yaml.Node{Kind: yaml.MappingNode}
	putChild(otherEntry, "url", plainScalar("http://127.0.0.1:9999/mcp"))
	putChild(servers, "other-server", otherEntry)
	return doc
}

func TestValidateTelegramBotToken(t *testing.T) {
	cases := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{"valid token", validTelegramToken, false},
		{"missing hash part", "123456789:", true},
		{"missing bot id", ":short-fake-token", true},
		{"no colon", "123456789AAEhBOweikshort-fake", true},
		{"non-digit bot id", "12a456789:short-fake-token", true},
		{"hash with invalid charset", "123456789:AAEh BOweik6ad9r", true},
		{"empty", "", true},
		{"whitespace only", "   ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTelegramBotToken(tc.token)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateTelegramBotToken(%q) error = %v, wantErr %v", tc.token, err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("error = %v, want wrapped domain.ErrInvalidInput", err)
			}
		})
	}
}

func TestValidateTelegramChatID(t *testing.T) {
	cases := []struct {
		name    string
		chatID  string
		wantErr bool
	}{
		{"valid private chat", validTelegramChatID, false},
		{"valid group id (negative)", "-1001234567890", false},
		{"empty", "", true},
		{"not numeric", "98765-4321a", true},
		{"whitespace inside", "987 654 321", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTelegramChatID(tc.chatID)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateTelegramChatID(%q) error = %v, wantErr %v", tc.chatID, err, tc.wantErr)
			}
		})
	}
}

func TestSetHermesTelegramOwnerBot_MergesWithoutClobbering(t *testing.T) {
	doc := seedTelegramFixture(t)

	if err := doc.SetHermesTelegramOwnerBot(validTelegramToken, validTelegramChatID, validOwnerPhone); err != nil {
		t.Fatalf("set telegram owner bot: %v", err)
	}

	root := doc.storage().root
	// Foreign top-level key preserved.
	if _, ok := childValue(root, "telemetry"); !ok {
		t.Error("foreign top-level key 'telemetry' was dropped")
	}
	servers, serversErr := childMapping(root, hermesServersSection)
	if serversErr != nil || servers == nil {
		t.Fatal("servers missing")
	}
	// Foreign sibling server preserved.
	if _, ok := childValue(servers, "other-server"); !ok {
		t.Error("foreign sibling server 'other-server' was dropped")
	}
	// The owned entry keeps url + headers AND gains env.
	entry, ok := childValue(servers, HermesServerName)
	if !ok || entry.Kind != yaml.MappingNode {
		t.Fatal("owned entry missing or not a map")
	}
	if _, ok := childValue(entry, hermesURLKey); !ok {
		t.Error("owned entry lost its url key")
	}
	if _, ok := childValue(entry, hermesHeadersKey); !ok {
		t.Error("owned entry lost its headers key")
	}
	env, err := childMapping(entry, hermesEnvKey)
	if err != nil || env == nil {
		t.Fatalf("env block missing or not a map")
	}
	for _, key := range []string{
		hermesTelegramTokenKey,
		hermesTelegramChatIDKey,
		hermesTelegramPhoneKey,
	} {
		if _, ok := childValue(env, key); !ok {
			t.Errorf("env key %s missing", key)
		}
	}
	// The owner phone env value must be a double-quoted scalar node (YAML 1.1
	// +54 trap applies to env values too).
	phoneNode, ok := childValue(env, hermesTelegramPhoneKey)
	if !ok || phoneNode.Style != yaml.DoubleQuotedStyle {
		t.Errorf("MCP_TELEGRAM_OWNER_PHONE is not double-quoted (got %v)", phoneNode)
	}
}

// TestSetHermesServer_PreservesTelegramEnv pins the reverse composition order:
// the Telegram flow first, then the server flow — the server entry replacement
// must carry the Telegram env block over instead of silently dropping it.
func TestSetHermesServer_PreservesTelegramEnv(t *testing.T) {
	doc := seedTelegramFixture(t)
	if err := doc.SetHermesTelegramOwnerBot(validTelegramToken, validTelegramChatID, validOwnerPhone); err != nil {
		t.Fatalf("telegram first: %v", err)
	}
	if err := doc.SetHermesServer("http://127.0.0.1:3001/mcp", validOwnerPhone); err != nil {
		t.Fatalf("server second: %v", err)
	}

	entry := hermesEntryFrom(t, doc)
	envNode, ok := childValue(entry, hermesEnvKey)
	if !ok || envNode.Kind != yaml.MappingNode {
		t.Fatal("env block dropped by SetHermesServer (silent data loss)")
	}
	for _, key := range []string{
		hermesTelegramTokenKey,
		hermesTelegramChatIDKey,
		hermesTelegramPhoneKey,
	} {
		if _, ok := childValue(envNode, key); !ok {
			t.Errorf("env key %s lost by SetHermesServer", key)
		}
	}
	if got := entryString(t, entry, hermesURLKey); got != "http://127.0.0.1:3001/mcp" {
		t.Errorf("url = %q, want the second-flow value", got)
	}
}

func TestSetHermesTelegramOwnerBot_ReRunIsIdempotent(t *testing.T) {
	doc := seedTelegramFixture(t)
	if err := doc.SetHermesTelegramOwnerBot(validTelegramToken, validTelegramChatID, validOwnerPhone); err != nil {
		t.Fatalf("first set: %v", err)
	}
	before, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal before: %v", err)
	}
	if err := doc.SetHermesTelegramOwnerBot(validTelegramToken, validTelegramChatID, validOwnerPhone); err != nil {
		t.Fatalf("second set: %v", err)
	}
	after, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("re-running the same Telegram values changed the document")
	}
}

func TestSetHermesTelegramOwnerBot_ValidationFailuresLeaveDocumentUntouched(t *testing.T) {
	doc := seedTelegramFixture(t)
	before, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal before: %v", err)
	}

	cases := []struct {
		name   string
		token  string
		chatID string
		phone  string
	}{
		{"invalid token", "not-a-token", validTelegramChatID, validOwnerPhone},
		{"invalid chat id", validTelegramToken, "abc", validOwnerPhone},
		{"invalid phone", validTelegramToken, validTelegramChatID, "555"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := doc.SetHermesTelegramOwnerBot(tc.token, tc.chatID, tc.phone); err == nil {
				t.Fatalf("expected a validation error")
			}
			after, err := marshalHermesDocument(doc)
			if err != nil {
				t.Fatalf("marshal after failed case: %v", err)
			}
			if string(before) != string(after) {
				t.Errorf("a validation failure mutated the document")
			}
		})
	}
}

func TestSetHermesTelegramOwnerBot_WritesAndPersistsAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, HermesConfigFileName)
	doc := seedTelegramFixture(t)

	if err := doc.SetHermesTelegramOwnerBot(validTelegramToken, validTelegramChatID, validOwnerPhone); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := WriteHermesConfig(path, doc); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if info.Mode().Perm() != os.FileMode(hermesFileMode) {
		t.Errorf("file mode = %v, want %v", info.Mode().Perm(), os.FileMode(hermesFileMode))
	}

	reloaded, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entry := hermesEntryFrom(t, reloaded)
	envNode, ok := childValue(entry, hermesEnvKey)
	if !ok || envNode.Kind != yaml.MappingNode || len(envNode.Content) != 6 {
		t.Errorf("env block missing or wrong shape after the write round trip")
	}
	if _, ok := childValue(entry, hermesURLKey); !ok {
		t.Error("owned entry lost url after the write round trip")
	}
	if _, ok := childValue(entry, hermesHeadersKey); !ok {
		t.Error("owned entry lost headers after the write round trip")
	}
	servers, err2 := childMapping(reloaded.storage().root, hermesServersSection)
	if err2 != nil {
		t.Fatalf("reload servers: %v", err2)
	}
	if _, ok := childValue(servers, "other-server"); !ok {
		t.Error("foreign sibling server lost after the write round trip")
	}
}

func TestMaskTelegramToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"valid token keeps only the bot id", validTelegramToken, "123456789:***"},
		{"malformed still never echoes", "whatever", "***"},
		{"empty", "", "***"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskTelegramToken(tc.in)
			if got != tc.want {
				t.Errorf("MaskTelegramToken(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, tc.in) && tc.in != "" {
				t.Errorf("masked value %q contains the full input", got)
			}
		})
	}
}

func TestSetHermesTelegramOwnerBot_RejectsScalarEntry(t *testing.T) {
	// R3-001 correction pin (review-f32febf9c0f9d664): a pre-existing scalar
	// entry can never host the env block; the merge must fail instead of
	// silently dropping the Telegram values.
	doc, err := LoadHermesConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("load empty document: %v", err)
	}
	servers, serversErr := childMapping(doc.storage().root, hermesServersSection)
	if serversErr != nil {
		t.Fatalf("seed servers: %v", serversErr)
	}
	putChild(servers, HermesServerName, plainScalar("not-a-map"))

	before, mErr := marshalHermesDocument(doc)
	if mErr != nil {
		t.Fatalf("marshal before: %v", mErr)
	}
	err = doc.SetHermesTelegramOwnerBot(validTelegramToken, validTelegramChatID, validOwnerPhone)
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("SetHermesTelegramOwnerBot(scalar entry) err = %v, want a semantic error", err)
	}
	after, mErr := marshalHermesDocument(doc)
	if mErr != nil {
		t.Fatalf("marshal after: %v", mErr)
	}
	if string(before) != string(after) {
		t.Error("a rejected merge mutated the document")
	}
}

// ── S11 unit B: the explicit "sin Telegram" removal path (TASK-2.4 step 2) ──
//
// Skipping the optional group PRESERVES a pre-existing MCP_TELEGRAM_* block
// (SetHermesServer carries it over by contract). Removing it is a separate,
// explicit flow: a business that configured the owner bot and retires it must be
// able to delete the block without hand-editing YAML — and without losing the
// url, the headers, a foreign env key or any other byte of the file.

// telegramSeededConfig writes seed to a throwaway config path and loads it back,
// asserting first that the document round-trips the seed VERBATIM: the removal
// contract is stated against bytes, so the fixture must be the file.
func telegramSeededConfig(t *testing.T, seed string) HermesDocument {
	t.Helper()
	path := filepath.Join(t.TempDir(), HermesConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("load seeded config: %v", err)
	}
	roundTrip, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal round trip: %v", err)
	}
	if string(roundTrip) != seed {
		t.Fatalf("fixture does not round-trip verbatim:\n got:\n%s\nwant:\n%s", roundTrip, seed)
	}
	return doc
}

// TestRemoveHermesTelegramOwnerBot_RemovesOnlyTheTelegramEnvKeys pins the
// byte-clean contract against a golden document: the three MCP_TELEGRAM_* keys
// are gone, and every other byte — the foreign top-level key, the sibling
// server, url, the quoted X-Caller-Id header and a foreign env key this flow
// does not own — survives verbatim.
func TestRemoveHermesTelegramOwnerBot_RemovesOnlyTheTelegramEnvKeys(t *testing.T) {
	seed := strings.Join([]string{
		"model: hermes-default",
		"telemetry:",
		"  enabled: true",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + validOwnerPhone + `"`,
		"    env:",
		`      ` + hermesTelegramTokenKey + `: "` + validTelegramToken + `"`,
		`      ` + hermesTelegramChatIDKey + `: "` + validTelegramChatID + `"`,
		`      ` + hermesTelegramPhoneKey + `: "` + validOwnerPhone + `"`,
		`      MCP_POLLERS_INTERVAL: "30"`,
		"",
	}, "\n")
	doc := telegramSeededConfig(t, seed)

	doc.RemoveHermesTelegramOwnerBot()

	got, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal after removal: %v", err)
	}
	want := strings.Join([]string{
		"model: hermes-default",
		"telemetry:",
		"  enabled: true",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + validOwnerPhone + `"`,
		"    env:",
		`      MCP_POLLERS_INTERVAL: "30"`,
		"",
	}, "\n")
	if string(got) != want {
		t.Errorf("document after removal =\n%s\nwant:\n%s", got, want)
	}
}

// TestRemoveHermesTelegramOwnerBot_DropsTheEnvBlockWhenItEmpties pins the shape
// of the common case: the env mapping only ever held the Telegram keys, so the
// removal leaves the entry exactly as a never-configured channel renders it
// ({url, headers}) instead of an empty `env: {}` shell.
func TestRemoveHermesTelegramOwnerBot_DropsTheEnvBlockWhenItEmpties(t *testing.T) {
	seed := strings.Join([]string{
		"mcp_servers:",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + validOwnerPhone + `"`,
		"    env:",
		`      ` + hermesTelegramTokenKey + `: "` + validTelegramToken + `"`,
		`      ` + hermesTelegramChatIDKey + `: "` + validTelegramChatID + `"`,
		`      ` + hermesTelegramPhoneKey + `: "` + validOwnerPhone + `"`,
		"",
	}, "\n")
	doc := telegramSeededConfig(t, seed)

	doc.RemoveHermesTelegramOwnerBot()

	got, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal after removal: %v", err)
	}
	want := strings.Join([]string{
		"mcp_servers:",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + validOwnerPhone + `"`,
		"",
	}, "\n")
	if string(got) != want {
		t.Errorf("document after removal =\n%s\nwant:\n%s", got, want)
	}
}

// TestRemoveHermesTelegramOwnerBot_IsIdempotentWhenAbsent pins the no-diff
// promise: removing a channel that was never configured (or removing twice)
// leaves today's-shape document byte-identical, so the flow can run on every
// host without touching unrelated bytes. The scalar entry is the fail-safe of a
// malformed config: it can never carry the block, so it is left untouched
// instead of being rewritten into a shape this flow does not own.
func TestRemoveHermesTelegramOwnerBot_IsIdempotentWhenAbsent(t *testing.T) {
	cases := map[string]string{
		"entry without env": strings.Join([]string{
			"mcp_servers:",
			"  mcp-appointments:",
			"    url: http://127.0.0.1:3000/mcp",
			"    headers:",
			`      X-Caller-Id: "` + validOwnerPhone + `"`,
			"",
		}, "\n"),
		"no owned entry": strings.Join([]string{
			"model: hermes-default",
			"mcp_servers:",
			"  openai:",
			"    url: https://api.example/mcp",
			"",
		}, "\n"),
		"scalar entry": strings.Join([]string{
			"mcp_servers:",
			"  mcp-appointments: not-a-map",
			"",
		}, "\n"),
		"empty document": "{}\n",
	}

	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			doc := telegramSeededConfig(t, seed)
			doc.RemoveHermesTelegramOwnerBot()
			doc.RemoveHermesTelegramOwnerBot()

			got, err := marshalHermesDocument(doc)
			if err != nil {
				t.Fatalf("marshal after removal: %v", err)
			}
			if string(got) != seed {
				t.Errorf("removal changed an absent-channel document:\n got:\n%s\nwant:\n%s", got, seed)
			}
		})
	}
}

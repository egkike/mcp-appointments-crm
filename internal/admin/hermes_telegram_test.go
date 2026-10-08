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
	validTelegramToken  = "123456789:AAEhBOweik6ad9r_QXMENQjcrGbqCr4K-4s"
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
	fields := doc.storage()
	fields["telemetry"] = map[string]any{"enabled": true}
	servers := fields[hermesServersSection].(map[string]any)
	servers["other-server"] = map[string]any{"url": "http://127.0.0.1:9999/mcp"}
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
		{"missing bot id", ":AAEhBOweik6ad9r_QXMENQjcrGbqCr4K-4s", true},
		{"no colon", "123456789AAEhBOweik6ad9rQXMENQjcrGbqCr4K4s", true},
		{"non-digit bot id", "12a456789:AAEhBOweik6ad9r_QXMENQjcrGbqCr4K-4s", true},
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

	fields := doc.storage()
	// Foreign top-level key preserved.
	if _, ok := fields["telemetry"]; !ok {
		t.Error("foreign top-level key 'telemetry' was dropped")
	}
	servers := fields[hermesServersSection].(map[string]any)
	// Foreign sibling server preserved.
	if _, ok := servers["other-server"]; !ok {
		t.Error("foreign sibling server 'other-server' was dropped")
	}
	// The owned entry keeps url + headers AND gains env.
	entry, ok := servers[HermesServerName].(map[string]any)
	if !ok {
		t.Fatalf("owned entry missing or not a map")
	}
	if _, ok := entry[hermesURLKey]; !ok {
		t.Error("owned entry lost its url key")
	}
	if _, ok := entry[hermesHeadersKey]; !ok {
		t.Error("owned entry lost its headers key")
	}
	env, ok := entry["env"].(map[string]any)
	if !ok {
		t.Fatalf("env block missing or not a map")
	}
	for _, key := range []string{
		"MCP_TELEGRAM_BOT_TOKEN",
		"MCP_TELEGRAM_ALLOWED_CHAT_ID",
		"MCP_TELEGRAM_OWNER_PHONE",
	} {
		if _, ok := env[key]; !ok {
			t.Errorf("env key %s missing", key)
		}
	}
	// The owner phone env value must be a double-quoted scalar node (YAML 1.1
	// +54 trap applies to env values too).
	phoneNode, ok := env["MCP_TELEGRAM_OWNER_PHONE"].(*yaml.Node)
	if !ok || phoneNode.Style != yaml.DoubleQuotedStyle {
		t.Errorf("MCP_TELEGRAM_OWNER_PHONE is not a double-quoted *yaml.Node (got %T)", env["MCP_TELEGRAM_OWNER_PHONE"])
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

	entry := doc.storage()[hermesServersSection].(map[string]any)[HermesServerName].(map[string]any)
	env, ok := entry["env"].(map[string]any)
	if !ok {
		t.Fatal("env block dropped by SetHermesServer (silent data loss)")
	}
	for _, key := range []string{
		"MCP_TELEGRAM_BOT_TOKEN",
		"MCP_TELEGRAM_ALLOWED_CHAT_ID",
		"MCP_TELEGRAM_OWNER_PHONE",
	} {
		if _, ok := env[key]; !ok {
			t.Errorf("env key %s lost by SetHermesServer", key)
		}
	}
	if entry[hermesURLKey] != "http://127.0.0.1:3001/mcp" {
		t.Errorf("url = %v, want the second-flow value", entry[hermesURLKey])
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
	reloadedFields := reloaded.storage()
	servers := reloadedFields[hermesServersSection].(map[string]any)
	entry := servers[HermesServerName].(map[string]any)
	env := entry["env"].(map[string]any)
	if len(env) != 3 {
		t.Errorf("env keys = %d, want 3", len(env))
	}
	if _, ok := servers["other-server"]; !ok {
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

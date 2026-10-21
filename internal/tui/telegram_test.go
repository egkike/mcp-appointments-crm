package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// Telegram owner-bot fixtures. They mirror the admin core fixtures: a valid
// token / chat id pair and the owner phone.
const (
	telegramToken  = "123456789:short-fake-token"
	telegramChatID = "987654321"
	telegramPhone  = "+5491100000001"
)

func TestTelegramFieldsValidate(t *testing.T) {
	cases := []struct {
		name    string
		fields  TelegramFields
		wantErr string
	}{
		{
			name:   "valid group",
			fields: NewTelegramFields(telegramToken, telegramChatID, telegramPhone),
		},
		{
			name:    "invalid token",
			fields:  NewTelegramFields("not-a-token", telegramChatID, telegramPhone),
			wantErr: "token del bot de Telegram",
		},
		{
			name:    "invalid chat id",
			fields:  NewTelegramFields(telegramToken, "abc", telegramPhone),
			wantErr: "chat id",
		},
		{
			name:    "invalid owner phone",
			fields:  NewTelegramFields(telegramToken, telegramChatID, "12"),
			wantErr: "no es válido",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fields.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want a rejection naming the field")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestTelegramFieldsSummaryMasksTheToken(t *testing.T) {
	fields := NewTelegramFields(telegramToken, telegramChatID, telegramPhone)

	summary := fields.Summary()

	if strings.Contains(summary, telegramToken) {
		t.Errorf("Summary() = %q, must never contain the full token", summary)
	}
	if !strings.Contains(summary, "123456789:***") {
		t.Errorf("Summary() = %q, want the masked token from admin.MaskTelegramToken", summary)
	}
	if !strings.Contains(summary, telegramChatID) || !strings.Contains(summary, telegramPhone) {
		t.Errorf("Summary() = %q, want the chat id and the owner phone", summary)
	}
}

// TestHermesWizardMasksTheTokenWhileTyping locks the token echo masking: the
// Telegram bot token is a credential, so the wizard must render the password
// mask while its step is focused and never the typed characters. The same
// textinput is reused by every step, so the test also pins that the next step
// goes back to plain echo (a stale EchoPassword would corrupt the chat id
// display).
func TestHermesWizardMasksTheTokenWhileTyping(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	d := newDriver(t, f)
	d.press("7")
	d.press("enter") // accept the prefilled server phone

	if d.model.form.index != 1 || d.model.form.steps[1].label != hermesTokenLabel {
		t.Fatalf("step = %d, want the Telegram token field", d.model.form.index)
	}

	d.typeText(telegramToken)
	tokenView := d.view()
	if strings.Contains(tokenView, telegramToken) {
		t.Errorf("token step leaks the typed secret\n%s", tokenView)
	}
	if !strings.Contains(tokenView, "•") {
		t.Errorf("token step = %q, want the password echo character", tokenView)
	}

	d.press("enter")
	d.typeText(telegramChatID)
	if chatView := d.view(); !strings.Contains(chatView, telegramChatID) {
		t.Errorf("chat id step = %q, want plain echo (the mask must not leak into the next step)", chatView)
	}
}

func TestTelegramFieldsSummaryNeverEchoesAMalformedToken(t *testing.T) {
	fields := NewTelegramFields("whatever", telegramChatID, telegramPhone)

	summary := fields.Summary()

	if strings.Contains(summary, "whatever") {
		t.Errorf("Summary() = %q, must not echo an unparseable token", summary)
	}
	if !strings.Contains(summary, "***") {
		t.Errorf("Summary() = %q, want the fail-closed mask", summary)
	}
}

// ── S11 unit B: the explicit "sin Telegram" removal path ──────────────────
//
// Skipping the optional group PRESERVES a configured block; retiring the channel
// is a THIRD, explicit path. TelegramRemoval() carries that intent, and the
// shared write chain deletes the block instead of merging it.

// TestTelegramRemovalSummaryStatesTheRemovedBlock pins the operator copy of the
// removal path: the channel reads as disabled and the summary states the block
// is gone, in the wording family of the skip line.
func TestTelegramRemovalSummaryStatesTheRemovedBlock(t *testing.T) {
	removal := TelegramRemoval()

	if !removal.Disabled() {
		t.Error("Disabled() = false, want true for the removal path")
	}
	if err := removal.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil (the removal collects no value)", err)
	}
	if got, want := removal.Summary(), "canal deshabilitado (bloque MCP_TELEGRAM eliminado)"; got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

// TestTelegramRemovalRejectsCollectedValues pins the fail-closed guard of a
// contradictory group: a removal that also carries a token or chat id would
// silently discard the values and report a removal, so it must be rejected
// instead of guessed.
func TestTelegramRemovalRejectsCollectedValues(t *testing.T) {
	contradictory := TelegramRemoval()
	contradictory.Token = telegramToken
	contradictory.ChatID = telegramChatID

	if err := contradictory.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want a rejection for a removal carrying Telegram values")
	}
}

// TestValidateTelegramRemovalAnswer pins the wizard-level rule of the removal
// question: "s" (with or without accent) removes, "n" and the empty answer keep
// the block, and anything else is re-prompted instead of interpreted.
func TestValidateTelegramRemovalAnswer(t *testing.T) {
	accepted := map[string]bool{
		"s": true, "sí": true, "si": true, "S": true,
		"n": false, "no": false, "": false,
	}
	for answer, wantRemove := range accepted {
		if err := validateTelegramRemovalAnswer(answer); err != nil {
			t.Errorf("validateTelegramRemovalAnswer(%q) error = %v, want it accepted", answer, err)
		}
		if got := telegramRemovalRequested(answer); got != wantRemove {
			t.Errorf("telegramRemovalRequested(%q) = %v, want %v", answer, got, wantRemove)
		}
	}

	if err := validateTelegramRemovalAnswer("quizás"); err == nil {
		t.Error("validateTelegramRemovalAnswer(\"quizás\") error = nil, want a re-prompt")
	}
	if telegramRemovalRequested("quizás") {
		t.Error("telegramRemovalRequested(\"quizás\") = true, want false for an uninterpretable answer")
	}
}

// TestApplyHermesConfigWithTelegramRemovesTheExistingBlock pins the write chain
// of the removal path end to end: a config that already carries the owner-bot
// block ends up WITHOUT it, while the url/headers and every foreign byte stay.
func TestApplyHermesConfigWithTelegramRemovesTheExistingBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hermes", admin.HermesConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seed := strings.Join([]string{
		"model: hermes-default",
		"mcp_servers:",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + telegramPhone + `"`,
		"    env:",
		`      MCP_TELEGRAM_BOT_TOKEN: "` + telegramToken + `"`,
		`      MCP_TELEGRAM_ALLOWED_CHAT_ID: "` + telegramChatID + `"`,
		`      MCP_TELEGRAM_OWNER_PHONE: "` + telegramPhone + `"`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	hermes := HermesConfig{Path: path, EndpointURL: "http://127.0.0.1:3000/mcp"}
	snippet, err := ApplyHermesConfigWithTelegram(hermes, telegramPhone, TelegramRemoval())
	if err != nil {
		t.Fatalf("ApplyHermesConfigWithTelegram() error = %v, want nil", err)
	}
	if snippet != "" {
		t.Errorf("snippet = %q, want the empty snippet of a successful write", snippet)
	}

	// #nosec G304 -- path is under a throwaway t.TempDir() home.
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read written config: %v", readErr)
	}
	if strings.Contains(string(content), "MCP_TELEGRAM") {
		t.Errorf("removal left the Telegram block behind\n%s", content)
	}
	for _, want := range []string{
		"model: hermes-default",
		"mcp-appointments",
		"http://127.0.0.1:3000/mcp",
		`X-Caller-Id: "` + telegramPhone + `"`,
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("removal dropped %q\n%s", want, content)
		}
	}
	if strings.Contains(string(content), "env:") {
		t.Errorf("removal left an empty env block\n%s", content)
	}
}

// TestApplyHermesConfigWithTelegramRejectsAContradictoryGroup pins the chain
// boundary: the field group is re-validated BEFORE the document is touched, so a
// contradictory group can never reach the removal branch (which would silently
// discard the values it carries). The caller still gets the manual fallback
// snippet, the same degradation of every failed chain.
func TestApplyHermesConfigWithTelegramRejectsAContradictoryGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hermes", admin.HermesConfigFileName)
	contradictory := TelegramRemoval()
	contradictory.ChatID = telegramChatID

	hermes := HermesConfig{Path: path, EndpointURL: "http://127.0.0.1:3000/mcp"}
	snippet, err := ApplyHermesConfigWithTelegram(hermes, telegramPhone, contradictory)
	if err == nil {
		t.Fatal("ApplyHermesConfigWithTelegram() error = nil, want the contradictory-group rejection")
	}
	if !strings.Contains(err.Error(), "no lleva token, chat id ni teléfono") {
		t.Errorf("error = %q, want the fail-closed group message", err)
	}
	if snippet == "" {
		t.Error("snippet = \"\", want the manual fallback the failed chain always renders")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("stat written config = %v, want no file after a rejected group", statErr)
	}
}

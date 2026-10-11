package tui

import (
	"strings"
	"testing"
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

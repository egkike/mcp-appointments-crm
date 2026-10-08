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

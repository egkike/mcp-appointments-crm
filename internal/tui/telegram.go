package tui

import (
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// TelegramFields is the Telegram owner-bot field group of the "Configurar
// Hermes" flow (ADR-0018 Decision 3b): the three values the gateway reads from
// the mcp_servers.mcp-appointments env block. It is a plain data carrier with
// no Bubble Tea dependency, so the wizard, the confirmation screen and the
// non-TTY console collect the same shape and apply the same rules.
//
// It owns no validation copy: Validate delegates to the reviewed admin
// validators, the single validation point both presentations already use.
type TelegramFields struct {
	Token      string
	ChatID     string
	OwnerPhone string
}

// NewTelegramFields wraps the three collected values. The owner phone is
// normally the active owner's account id (the wizard prefills it), but it stays
// editable because the Telegram owner may use a different phone.
func NewTelegramFields(token, chatID, ownerPhone string) TelegramFields {
	return TelegramFields{Token: token, ChatID: chatID, OwnerPhone: ownerPhone}
}

// Validate applies the admin validators in field order, so the operator reads
// the first offending field as a Spanish message. It never echoes the offending
// value: the messages come from the reviewed core.
func (f TelegramFields) Validate() error {
	if err := admin.ValidateTelegramBotToken(f.Token); err != nil {
		return err
	}
	if err := admin.ValidateTelegramChatID(f.ChatID); err != nil {
		return err
	}
	return admin.ValidatePhone(f.OwnerPhone)
}

// Summary renders the operator-facing summary of the field group. The bot token
// is masked through admin.MaskTelegramToken, so the full secret can never reach
// a confirmation, a success screen or the console output.
func (f TelegramFields) Summary() string {
	return fmt.Sprintf("token %s · chat %s · teléfono %s",
		admin.MaskTelegramToken(f.Token), f.ChatID, f.OwnerPhone)
}

// ApplyHermesConfigWithTelegram runs the option 7 write chain over the reviewed
// core: load the existing config, merge the server entry (SetHermesServer),
// merge the Telegram owner-bot env block (SetHermesTelegramOwnerBot) and write
// the document atomically. The two merges touch disjoint parts of the same
// entry, so the admin core guarantees they compose in either order.
//
// It is exported because both presentations run it: the Bubble Tea
// hermesConfigCmd and the line-based console runConfigureHermesFlow. The
// reviewed admin.ApplyHermesConfig predates the Telegram step and merges only
// the server entry, so this composition adds the second merge without
// reimplementing any YAML or atomic-write work.
//
// On success it returns an empty snippet and a nil error. On failure it returns
// the semantic error that broke the chain plus the exact server-entry snippet
// the writer would emit — empty when it cannot be rendered — so both callers
// degrade to the same ADR-0017 Decision 1 manual fallback.
func ApplyHermesConfigWithTelegram(hermes HermesConfig, serverPhone string, telegram TelegramFields) (snippet string, err error) {
	doc, err := admin.LoadHermesConfig(hermes.Path)
	if err == nil {
		err = doc.SetHermesServer(hermes.EndpointURL, serverPhone)
	}
	if err == nil {
		err = doc.SetHermesTelegramOwnerBot(telegram.Token, telegram.ChatID, telegram.OwnerPhone)
	}
	if err == nil {
		err = admin.WriteHermesConfig(hermes.Path, doc)
	}
	if err == nil {
		return "", nil
	}

	snippet, snippetErr := admin.RenderHermesSnippet(hermes.EndpointURL, serverPhone)
	if snippetErr != nil {
		snippet = ""
	}
	return snippet, err
}

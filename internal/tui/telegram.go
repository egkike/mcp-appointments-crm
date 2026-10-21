package tui

import (
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// TelegramFields is the Telegram owner-bot field group of the "Configurar
// Hermes" flow (ADR-0018 Decision 3b): the three values the gateway reads from
// the mcp_servers.mcp-appointments env block. It is a plain data carrier with
// no Bubble Tea dependency, so the wizard, the confirmation screen and the
// non-TTY console collect the same shape and apply the same rules.
//
// The group is OPTIONAL (S11, owner decision 2026-10-19 "skip + deshabilitar"):
// the official channels are the business WhatsApp number and the local Hermes
// chat, and Telegram is an additive owner-only channel. An empty group is the
// operator's skip and writes the server entry without the Telegram block; a
// HALF-filled group keeps the required-pair semantics the flow always had.
//
// It owns no validation copy: Validate delegates to the reviewed admin
// validators, the single validation point both presentations already use.
type TelegramFields struct {
	Token      string
	ChatID     string
	OwnerPhone string
}

// telegramDisabledSummary is the operator-facing summary of a skipped group. It
// states the channel is disabled instead of rendering three empty values, which
// would read as a broken configuration rather than a decision.
const telegramDisabledSummary = "no configurado (canal deshabilitado)"

// NewTelegramFields wraps the three collected values. The owner phone is
// normally the active owner's account id (the wizard prefills it), but it stays
// editable because the Telegram owner may use a different phone.
func NewTelegramFields(token, chatID, ownerPhone string) TelegramFields {
	return TelegramFields{Token: token, ChatID: chatID, OwnerPhone: ownerPhone}
}

// Disabled reports whether the operator left the whole group empty: no bot token
// and no chat id, i.e. the Telegram owner-bot channel stays disabled (S11).
//
// The token and the chat id are a pair, so a group carrying only ONE of them is
// NOT disabled: it is an incomplete pair that Validate rejects with the same
// message the required flow always produced. The owner phone does not enter the
// predicate — a phone alone never enables a channel whose credential is the bot
// token, and the wizard only collects it after the token is present.
func (f TelegramFields) Disabled() bool {
	return strings.TrimSpace(f.Token) == "" && strings.TrimSpace(f.ChatID) == ""
}

// Validate applies the admin validators in field order, so the operator reads
// the first offending field as a Spanish message. It never echoes the offending
// value: the messages come from the reviewed core.
//
// Skip semantics (S11): the empty group is a skip, not a rejection, so it
// validates with no error. Every other group keeps the required-pair behavior
// byte for byte.
func (f TelegramFields) Validate() error {
	if f.Disabled() {
		return nil
	}
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
// a confirmation, a success screen or the console output. A disabled group
// renders the disabled-channel sentence instead of an empty group.
func (f TelegramFields) Summary() string {
	if f.Disabled() {
		return telegramDisabledSummary
	}
	return fmt.Sprintf("token %s · chat %s · teléfono %s",
		admin.MaskTelegramToken(f.Token), f.ChatID, f.OwnerPhone)
}

// optionalTelegramToken is the wizard validator of the token step: an
// empty answer is the operator skipping the optional channel, any other answer
// must pass the reviewed core shape check. It is a step-level rule only — the
// pair is still validated whole by TelegramFields.Validate before the write.
func optionalTelegramToken(answer string) error {
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	return admin.ValidateTelegramBotToken(answer)
}

// ApplyHermesConfigWithTelegram runs the option 7 write chain over the reviewed
// core: load the existing config, merge the server entry (SetHermesServer),
// merge the Telegram owner-bot env block (SetHermesTelegramOwnerBot) and write
// the document atomically. The two merges touch disjoint parts of the same
// entry, so the admin core guarantees they compose in either order.
//
// Skip semantics (S11): a TelegramFields group that Disabled() skips the env
// merge entirely, so the entry is written as the server entry ALONE —
// `mcp_servers.mcp-appointments: {url, headers: {X-Caller-Id}}` with no `env`
// block — and the operator reads "Telegram: no configurado (canal deshabilitado)"
// in the summary. That shape is the one Hermes already boots with: the server
// tolerates a missing MCP_TELEGRAM_* block at runtime (verified 2026-10-19,
// ADR-0018 §3b treats Telegram as an additive channel), so no config ever
// REQUIRED the block — only this chain's unconditional Telegram merge did. A
// previously written env block is preserved by SetHermesServer, so skipping
// never drops a bot the operator already configured; REMOVING it is a separate
// flow, not part of the skip path.
//
// It is exported because both presentations run it: the Bubble Tea
// hermesConfigCmd and the line-based console runConfigureHermesFlow. The
// reviewed admin.ApplyHermesConfig predates the Telegram step and merges only
// the server entry, so this composition adds the optional second merge without
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
	if err == nil && !telegram.Disabled() {
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

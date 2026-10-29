package tui

import (
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// TelegramFields is the Telegram owner-bot field group of the "Configurar
// Hermes" flow (ADR-0018 Decision 3b): the three values the gateway reads from
// the mcp_servers.mcp-appointments env block. It is a plain data carrier with
// no Bubble Tea dependency, so the wizard, the confirmation screen and the
// non-TTY console collect the same shape and apply the same rules.
//
// The group is OPTIONAL (S11, owner decision 2026-10-19 "skip + deshabilitar"):
// the official channels are the business WhatsApp number and the local Hermes
// chat, and Telegram is an additive owner-only channel. It has three exclusive
// shapes: a complete group configures (or replaces) the channel, an empty group
// SKIPS it and preserves whatever block the config already had, and the explicit
// removal path (Remove) DELETES an existing block. A HALF-filled group keeps the
// required-pair semantics the flow always had.
//
// It owns no validation copy: Validate delegates to the reviewed admin
// validators, the single validation point both presentations already use.
type TelegramFields struct {
	Token      string
	ChatID     string
	OwnerPhone string
	// Remove marks the explicit "sin Telegram" path (S11 unit B): the operator
	// retires the channel, so the write DELETES an existing MCP_TELEGRAM_* block
	// instead of merging one. A skipping group leaves the field false and
	// therefore preserves whatever block the config already carries.
	Remove bool
}

// telegramDisabledSummary is the operator-facing summary of a skipped group. It
// renders the decision instead of three empty values, which would read as a
// broken configuration. It is deliberately neutral about the file: the skip path
// never loads or verifies whether a MCP_TELEGRAM_* block is already on disk, so
// any copy claiming the channel state would be an unverified postcondition (B15
// C3). "Sin cambios" is true in both worlds: a surviving block stays as it was,
// and an absent one stays absent.
const telegramDisabledSummary = "omitido (sin cambios en el bloque de Telegram)"

// telegramRemovalSummary is the operator-facing summary of the removal path: the
// channel is disabled AND the block is gone. It is a post-condition of the write
// (an idempotent removal guarantees the entry carries no MCP_TELEGRAM_* key), not
// a claim about the previous state, so it stays accurate when there was nothing
// to delete.
const telegramRemovalSummary = "canal deshabilitado (bloque MCP_TELEGRAM eliminado)"

// NewTelegramFields wraps the three collected values. The owner phone is
// normally the active owner's account id (the wizard prefills it), but it stays
// editable because the Telegram owner may use a different phone.
func NewTelegramFields(token, chatID, ownerPhone string) TelegramFields {
	return TelegramFields{Token: token, ChatID: chatID, OwnerPhone: ownerPhone}
}

// TelegramRemoval builds the field group of the explicit "sin Telegram" path
// (S11 unit B): no value is collected and the shared write chain removes the
// MCP_TELEGRAM_* block. It is the third choice of option 7, next to configuring
// the channel and skipping it (which preserves an existing block).
func TelegramRemoval() TelegramFields {
	return TelegramFields{Remove: true}
}

// Disabled reports whether the operator left the whole group empty: no bot token
// and no chat id, i.e. the Telegram owner-bot channel stays disabled (S11).
//
// The removal path is disabled by definition — it collects no value — so it keys
// the same way: no channel is written. Which of the two disabled shapes applies
// (preserve vs delete) is the Remove flag, decided by the operator's explicit
// answer.
//
// The token and the chat id are a pair, so a group carrying only ONE of them is
// NOT disabled: it is an incomplete pair that Validate rejects with the same
// message the required flow always produced. The owner phone does not enter the
// predicate — a phone alone never enables a channel whose credential is the bot
// token, and the wizard only collects it after the token is present.
func (f TelegramFields) Disabled() bool {
	if f.Remove {
		return true
	}
	return strings.TrimSpace(f.Token) == "" && strings.TrimSpace(f.ChatID) == ""
}

// Validate applies the admin validators in field order, so the operator reads
// the first offending field as a Spanish message. It never echoes the offending
// value: the messages come from the reviewed core.
//
// Skip semantics (S11): the empty group is a skip, not a rejection, so it
// validates with no error. Every other group keeps the required-pair behavior
// byte for byte.
//
// Removal semantics (S11 unit B): the removal path collects no value, so it
// validates clean. A removal that ALSO carries a token, a chat id or a phone is
// contradictory: ignoring those values would persist nothing while reporting a
// removal, so the group fails closed instead of guessing which half the operator
// meant.
func (f TelegramFields) Validate() error {
	if f.Remove {
		if strings.TrimSpace(f.Token) != "" || strings.TrimSpace(f.ChatID) != "" || strings.TrimSpace(f.OwnerPhone) != "" {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el retiro del canal Telegram no lleva token, chat id ni teléfono: elegí configurar el canal o retirarlo, no las dos cosas",
			}
		}
		return nil
	}
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
// renders the disabled-channel sentence instead of an empty group, and the
// removal path says the block was deleted instead of claiming it was never
// configured — the two disabled shapes must never read alike.
func (f TelegramFields) Summary() string {
	if f.Remove {
		return telegramRemovalSummary
	}
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

// validateTelegramRemovalAnswer is the wizard/console rule of the explicit
// removal question (S11 unit B): "s" confirms the removal, "n" and the empty
// answer keep whatever block the config has. Anything else is re-prompted
// instead of interpreted — a typo must never delete a configured channel.
func validateTelegramRemovalAnswer(answer string) error {
	if telegramRemovalAnswer(answer) == telegramAnswerInvalid {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "respondé \"s\" o \"n\"",
		}
	}
	return nil
}

// telegramRemovalRequested reports whether the answer asks for the removal. An
// uninterpretable answer never removes anything (fail-closed).
func telegramRemovalRequested(answer string) bool {
	return telegramRemovalAnswer(answer) == telegramAnswerRemove
}

// telegramAnswer is the normalized reading of the removal question.
type telegramAnswer int

const (
	telegramAnswerInvalid telegramAnswer = iota
	telegramAnswerKeep
	telegramAnswerRemove
)

// telegramRemovalAnswer normalizes the answer once, so the validator and the
// consumer can never disagree about what was read.
func telegramRemovalAnswer(answer string) telegramAnswer {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "s", "si", "sí":
		return telegramAnswerRemove
	case "", "n", "no":
		return telegramAnswerKeep
	default:
		return telegramAnswerInvalid
	}
}

// ApplyHermesConfigWithTelegram runs the option 7 write chain over the reviewed
// core: load the existing config, merge the server entry (SetHermesServer),
// merge the Telegram owner-bot env block (SetHermesTelegramOwnerBot) and write
// the document atomically. The two merges touch disjoint parts of the same
// entry, so the admin core guarantees they compose in either order.
//
// Skip semantics (S11): a TelegramFields group that Disabled() skips the env
// merge entirely, so a previously written env block survives untouched and the
// entry keeps whatever Telegram state it already had — and the operator reads
// "Telegram: omitido (sin cambios en el bloque de Telegram)" in the summary.
// That neutral copy (B15 C3) makes no file claim the skip never verified. That
// shape is the one Hermes already boots with: the server
// tolerates a missing MCP_TELEGRAM_* block at runtime (verified 2026-10-19,
// ADR-0018 §3b treats Telegram as an additive channel), so no config ever
// REQUIRED the block — only this chain's unconditional Telegram merge did. A
// previously written env block is preserved by SetHermesServer, so skipping
// never drops a bot the operator already configured.
//
// Removal semantics (S11 unit B): the operator choosing "Retirar el canal
// Telegram del bot" marks the group with Remove, and the chain then calls
// admin.RemoveHermesTelegramOwnerBot instead of the merge — AFTER SetHermesServer
// (which carries the previous env block over), so the documented removal contract
// applies to exactly the bytes the two merges would have written. The operator
// reads "Telegram: canal deshabilitado (bloque MCP_TELEGRAM eliminado)".
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
//
// The field group is re-validated HERE, before the document is loaded: the
// wizard and the console validate their own answers, but this is the boundary
// every write passes through, and only a validated group is safe to dispatch —
// a contradictory group (a removal carrying values) would otherwise reach the
// removal branch and silently discard them.
func ApplyHermesConfigWithTelegram(hermes HermesConfig, serverPhone string, telegram TelegramFields) (snippet string, err error) {
	if err := telegram.Validate(); err != nil {
		return hermesFallbackSnippet(hermes, serverPhone), err
	}

	doc, err := admin.LoadHermesConfig(hermes.Path)
	if err == nil {
		err = doc.SetHermesServer(hermes.EndpointURL, serverPhone)
	}
	if err == nil {
		switch {
		case telegram.Remove:
			doc.RemoveHermesTelegramOwnerBot()
		case !telegram.Disabled():
			err = doc.SetHermesTelegramOwnerBot(telegram.Token, telegram.ChatID, telegram.OwnerPhone)
		}
	}
	if err == nil {
		err = admin.WriteHermesConfig(hermes.Path, doc)
	}
	if err == nil {
		return "", nil
	}

	return hermesFallbackSnippet(hermes, serverPhone), err
}

// hermesFallbackSnippet renders the manual fallback of ADR-0017 Decision 1: the
// exact server-entry block the writer would emit, or "" when it cannot be
// rendered. Both the validation failure and the write failure of the chain
// degrade to this same snippet, so the two paths cannot report differently.
func hermesFallbackSnippet(hermes HermesConfig, serverPhone string) string {
	snippet, err := admin.RenderHermesSnippet(hermes.EndpointURL, serverPhone)
	if err != nil {
		return ""
	}
	return snippet
}

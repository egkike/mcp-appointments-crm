# Spec: hermes-config-tui

> Reference: `openspec/changes/feat-whatsapp-bot/proposal.md` D1/D3, `design.md` §4, ADR-0017 (opción 7), ADR-0018 Decision 3b (bot privado de Telegram, inyección estática del teléfono del owner)
> Change: feat-whatsapp-bot · Status: NEW (extends the behavior delivered by ADR-0017 / v0.6.0; no capability spec exists under `openspec/specs/`)

## Purpose

The private Telegram owner-bot (ADR-0018 Decision 3b) needs three values in `~/.hermes/config.yaml`: bot token, allowlist chat id and the owner's real phone, which is the source of truth for the static `X-Caller-Id` injection (Telegram does not expose sender phones to bots). Option 7 already bootstraps that YAML safely; this capability adds a Telegram owner-bot field group so the owner never hand-edits YAML, never hits the YAML 1.1 `+54…` integer trap, and never breaks unrelated keys.

## Requirements

### Requirement: Option 7 collects the Telegram owner-bot fields

The "Configurar Hermes" flow MUST collect three values: bot token, allowlist chat id and owner phone. The owner phone MUST be prefilled from the active owner in `accounts` when one exists and MUST stay editable. The field group MUST be reachable from the existing option 7 entry point in both the Bubble Tea and the non-TTY console presentations, applying the same validations in both.

#### Scenario: Field group is reachable from option 7

- GIVEN an operator running `mcp-server admin tui` with a valid database
- WHEN the operator selects option 7
- THEN the flow MUST offer the Telegram owner-bot field group alongside the existing Hermes bootstrap fields

#### Scenario: Non-TTY parity

- GIVEN a non-TTY execution using the console flow
- WHEN the operator completes the onboarding prompts
- THEN the same three values MUST be collected with the same validation rules

### Requirement: Every collected value is validated before the write

The bot token MUST be non-empty and MUST match `^\d{8,12}:[A-Za-z0-9_-]{30,40}$`. The allowlist chat id MUST match `^-?\d{5,20}$`. The owner phone MUST satisfy `entity.Client.HasValidPhone` (optional leading `+`, at least 4 digits — 5 with the `+`; the entity validator has no upper bound) and MUST carry at most 15 digits (E.164 maximum), enforced by this field's own validator. A validation failure MUST block the write, MUST show a Spanish message naming the field, and MUST NOT echo the offending value.

#### Scenario: Malformed values are rejected

- GIVEN the operator enters `not-a-token` as the token, `abc` as the chat id, or `12` as the owner phone
- WHEN the field is validated
- THEN the flow MUST refuse to advance with a Spanish message naming that field and MUST NOT modify the configuration file

### Requirement: The write merges into the existing document and preserves foreign keys

The flow MUST merge the three values into the existing `mcp_servers.mcp-appointments` entry using node-level YAML decoding, preserving every unrelated key — inside that entry or anywhere else in the document. When the file or entry is absent, the flow MUST create only the required subtree, as option 7 does today.

#### Scenario: Unrelated keys survive the write

- GIVEN a config containing other `mcp_servers` entries, unrelated top-level keys and other keys inside `mcp_servers.mcp-appointments`
- WHEN the operator completes the field group
- THEN every pre-existing key MUST remain with its original value and the three new values MUST be present under `mcp_servers.mcp-appointments`

#### Scenario: Missing file is created minimally

- GIVEN no `~/.hermes/config.yaml` exists
- WHEN the operator completes the flow
- THEN the file MUST be created containing only the minimum structure required for the entry

### Requirement: Values are written atomically with restricted permissions and quoting

The write MUST target a temporary sibling file created with mode `0600`, MUST be flushed before being renamed over the target, and MUST leave the original untouched if any step fails. Phone-like values MUST be emitted as quoted strings so YAML 1.1 does not parse a leading `+` as an integer. On success the file mode MUST be `0600`.

#### Scenario: Phone is quoted and permissions restricted

- GIVEN an owner phone `+5491100999999` and a successful write
- WHEN the file is written
- THEN the value MUST appear quoted (`"+5491100999999"`) and not as a bare scalar, and the file mode MUST be `0600`

#### Scenario: Failure leaves the original intact

- GIVEN a write step that fails before the rename
- WHEN the flow reports the failure
- THEN the original `~/.hermes/config.yaml` MUST be unchanged and the operator MUST see a Spanish error message

### Requirement: Secrets are never displayed or logged

The bot token MUST NOT be echoed, logged, or included in error or audit output. Non-secret values MAY be shown masked (owner phone to its last 4 digits). Failure messages MUST name the field, never the entered value.

#### Scenario: Token is not echoed

- GIVEN an operator entering a valid bot token
- WHEN the flow completes and prints its summary
- THEN the summary MUST NOT contain the token value

#### Scenario: Failure messages name fields, not values

- GIVEN any validation failure on the token
- WHEN the error is displayed or logged
- THEN it MUST name the field and MUST NOT contain the entered value

### Requirement: Cancel leaves the configuration untouched

Cancelling or going back before the final confirmation MUST NOT modify the file on disk.

#### Scenario: Cancel is a no-op

- GIVEN an operator who entered the three values but cancels before confirming
- WHEN the flow exits
- THEN `~/.hermes/config.yaml` MUST be byte-identical to its state before the flow

## Notes

- Telegram-side allowlist enforcement happens in the gateway before any tool call reaches the server (ADR-0018 Decision 3b); this capability only persists the values the gateway reads.
- The stored phone is the source of truth for the static `X-Caller-Id` injection; it MUST NOT be treated as a role source — roles always come from `accounts`/`clients`.
- The existing dependency `gopkg.in/yaml.v3` covers node-level merging; no new dependency is introduced.

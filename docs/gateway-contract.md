# Gateway Contract — Per-Sender Identity for the Business Channel

> **Applies to:** the Hermes-side messaging gateway (WhatsApp primary, Telegram alternative) that fronts the MCP server.
> **Status:** delivered by `feat-whatsapp-bot` (Phases 1–5, server side). This document is the Hermes-side protocol that the delivered server contract enables.
> **References:** [ADR-0018](architecture/0018-communication-channels.md) (frozen channel model) · `openspec/changes/feat-whatsapp-bot/specs/client-registration/spec.md` (registration contract) · [ADR-0009](architecture/0009-authorization-model.md) (authorization) · [ADR-0017](architecture/0017-hermes-config-tui.md) (TUI option 7).

The server never changes for channels: the gateway reads the sender of each inbound message and injects it as `X-Caller-Id`. The role always comes from `accounts` / `clients` — the channel transports an identity, never a role (ADR-0018 D2).

## 1. Identity injection

| Rule | Value |
|---|---|
| Header | `X-Caller-Id: <sender phone>` |
| Source | the `from` field of the inbound message |
| Value | injected **verbatim** — no normalization, no added/removed characters |
| Scope | per sender, per request: never a static conversation-level id where the sender can vary |
| Endpoint | the MCP server, strictly over `127.0.0.1:3000` (loopback invariant, ADR-0018) |
| Missing/empty header | fails before any handler: HTTP `401` → JSON-RPC `-32000` `"no se proporcionó X-Caller-Id"`; no caller is invented |
| Byte stability | the stored `id` must equal the exact bytes the gateway sends next time — the resolver looks up `clients.id == phone` |

## 2. Server responses the gateway must interpret

| JSON-RPC code | Meaning | Exact server message |
|---|---|---|
| `-32000` | Auth failure. Unknown sender, disabled account, or absent header. | `"no te reconozco. Por favor regístrate primero."` (unknown) · `"tu cuenta está deshabilitada. Contacta al administrador."` (inactive) · `"no se proporcionó X-Caller-Id"` |
| `-32002` | Business/semantic rejection (`*domain.SemanticError`). The message is person-facing Spanish. | Registration rejections in §4.1: invalid phone, accounts collision, legacy id, rate limit |
| `-32603` | Internal error (SQL, driver, filesystem). Never surfaces internals. | `"error interno del servidor"` — report to the owner, do not retry blindly |

## 3. The registration loop (unknown sender)

```
inbound message → gateway reads `from` → injects X-Caller-Id (per sender)
  → original tool call
      ├─ success                      → proceed
      └─ -32000 "no te reconozco"     → 1. ask the sender for a display name (OPTIONAL — must not block)
                                          2. call register_client {display_name?}   (header already carries the phone)
                                          3. retry the original tool call ONCE
                                          4. retry still fails → report to the owner; do NOT loop
```

| Step | Contract |
|---|---|
| Trigger | only `-32000` with the unknown-sender message; a disabled account or a missing header must not register |
| Name | optional: on refusal, continue with no `display_name`; the server stores `Cliente {phone}` |
| Retry | exactly once. A second `-32000` after a successful `register_client` is a defect, not a loop |
| Non-retryable | `-32002` and `-32603` are surfaced to the person/owner as-is — never fed back into `register_client` |

## 4. `register_client` tool contract

| Aspect | Value |
|---|---|
| Input | `{"display_name": "<string, optional>"}` — **`display_name` only**; a `phone`/`caller_id`/`id`/unknown key is rejected by the SDK schema (`additionalProperties: false`) |
| Identity | from `X-Caller-Id` only — the body can never set the phone |
| Output | `{"client_id": "<header phone>", "display_name": "<stored name>", "created": true|false}` — `created=false` when the phone was already registered |
| Creation | `clients.id == phone` and `clients.phone == phone`, byte-identical to the header; no UUID is ever generated |
| Accounts collision | a phone already in `accounts` (active or inactive) is rejected — `-32002` `"este número ya pertenece a una cuenta del negocio. Contactá al administrador."` |
| Legacy id | a client row whose `id` is a UUID (not the phone) is rejected — `-32002` `"este número ya está registrado con un identificador incompatible; contactá al negocio."` |
| Phone format | optional `+`, 4–15 digits (E.164 maximum). Otherwise `-32002` `"el teléfono no tiene un formato válido (se esperan entre 4 y 15 dígitos, con + opcional)."` |
| Rate limit | per-phone, `MCP_REGISTRATION_RATE_LIMIT` (default 10/hour; `0` disables registration entirely, fail-closed). Exhaustion → `-32002` `"alcanzaste el límite de registros automáticos. Probá de nuevo más tarde o pedile al negocio que te registre."` |
| Audit | server-side only; the phone is masked to its last 4 digits in every event |

### 4.1 Rejections are gateway-visible outcomes

When `register_client` answers `-32002`, the gateway MUST surface the Spanish message to the person (translated, never swallowed and never retried as if transient). Accounts collision and rate limit are expected business states, not bugs.

## 5. Telegram: `request_contact` before the first tool call

Telegram does not expose the sender's phone to a bot. Before the first tool call of a Telegram conversation, the gateway MUST ask the sender to share their contact (`request_contact`) and use that number as `X-Caller-Id`. This friction is exactly why WhatsApp is the primary channel (ADR-0018 Consequences). The Telegram owner-bot is a separate, owner-only path that injects a static phone from config (§7) — it does not do per-sender resolution.

## 6. Scenarios the contract must cover

| # | Sender | Session number | Session JID | How the sender resolves |
|---|---|---|---|---|
| A | Owner writing from a personal number ≠ the business number | business number | — | incoming JID matches `accounts` → `Role = owner` naturally; no registration |
| B | Owner whose number **is** the business number (single-line business, WhatsApp self-chat) | same as the owner account | self-chat JID | self-chat sender MUST be treated as a valid sender and resolved through `accounts` → `owner` |
| C | Any unknown phone (first-time client) | business number | — | `-32000` → ask name (optional) → `register_client` → retry once → `Role = client` |

A self-chat sender (B) is a valid sender: treat it like any other `from` and let `accounts` assign the role. Scenario A and B share the identical JID resolution chain (`accounts` → `clients.phone` → unknown).

## 7. Operator runbook

### 7.1 Transfer Ownership to the real owner phone

The demo owner (`+5491100000000`) must be replaced by the real phone before production. This is an existing TUI capability — no code change:

1. `mcp-server admin tui`
2. Select **Transferir ownership** and enter the real owner phone (and display name).
3. The single-owner invariant holds across the two-step transfer; the successor becomes the resolvable `owner`.

### 7.2 Set the business channel to `whatsapp`

The Telegram/WhatsApp choice is a documented operator step using existing tools:

1. Call `update_business_profile` with `messenger_platform: "whatsapp"` and `messenger_id` set to the business number.
2. Verify with `get_business_profile` that `messenger_platform` reads `whatsapp` and `messenger_id` matches the business number.

### 7.3 Registration rate limit

| Key | `MCP_REGISTRATION_RATE_LIMIT` |
|---|---|
| Default | `10` (per phone, per hour) |
| `0` | disables auto-registration entirely (fail-closed) |
| Malformed/negative | falls back to the default — a typo never opens writes |
| State | in-memory, single process; a restart resets the window (accepted: loopback, operator-initiated restarts) |

### 7.4 Telegram owner-bot env keys (written by TUI option 7)

TUI option 7 writes these into `mcp_servers.mcp-appointments.env` in `~/.hermes/config.yaml`:

| Key | Purpose |
|---|---|
| `MCP_TELEGRAM_BOT_TOKEN` | owner-bot token (secret; never echoed or logged) |
| `MCP_TELEGRAM_ALLOWED_CHAT_ID` | the only chat allowed to talk to the owner bot |
| `MCP_TELEGRAM_OWNER_PHONE` | the owner's real phone, injected **statically** as `X-Caller-Id` (ADR-0018 D3b) |

> The Hermes-side consumption spelling of these keys is the gateway implementer's reconciliation point: the TUI writes the server-side config keys above; the gateway must read the same names to perform the static owner injection.

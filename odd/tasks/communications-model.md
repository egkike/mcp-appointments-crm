# Feature: communications-model

**Date**: 2026-09-27
**Type**: docs (ADR + PRD alignment) — light lane, direct to main after structural readback
**TDD**: OFF (docs only)

## Goal

Freeze the product-wide **communication channels model** for the MCP server and its
Hermes agent, resolved with the owner on 2026-09-27 after the original plan was
interviewed against code (auth/resolver.go, schema CHECK, PRD §3.8.9).

The three product decisions:

1. **Single business channel** — the business has exactly one phone/account, used with
   WhatsApp **or** Telegram (schema already enforces this):
   `business_profile.messenger_platform CHECK IN ('whatsapp','telegram')`.
2. **Per-sender identity over the business channel** — clients, professionals and owner
   all message the business number from their own phone; the gateway injects the
   sender-identity `X-Caller-Id`; existing RBAC (`CallerResolver` chains
   `accounts` → `clients`) applies the right role per role.
3. **Owner-only non-messenger channels** — only the owner bypasses the business
   number:
   - SSH + `mcp-server hermes chat` (local, gatekeeper = OS login; **the
     `MCP_CALLER_ID` multi-user override for staff is RETIRED** — ADR-0012
     multi-user amendment);
   - **Bot privado de Telegram** (owner-only allowlist). Identity here is injected
     statically (Telegram hides sender phone behind a user ID): the MCP config holds
     the owner's real phone and the gateway only forwards chats from the allowlisted
     chat. This channel is new in the backlog (not yet implemented).

An additional product rule resolved during the interview:
- **Unknown caller policy → auto-register as client**: a first-time phone writing the
  business channel is created via `get_or_create_client` with that phone
  (`rol client`, self-service only). Today the code answers 401 — this gap is
  now an explicit future spec item for the bot feature.
- **Telegram per-sender friction note**: Telegram never exposes
  the sender's phone to a bot (anonymous user ID). Resolving a sender there requires
  `request_contact` at first contact. WhatsApp (JID == phone) is the primary;
  Telegram is the supported alternative.

## Scope (docs only)

- **New**: `docs/architecture/0018-communication-channels.md` — ADR-0018 with the
  3-channel model + the 2 product rules above + retirement of ADR-0012 multi-user.
- **Edit**: PRD §3.8.9 table (chat row: owner-only, no staff override),
  new §3.8.10 “Communication channels” summary, §7 Fase N backlog item for the bot
  gains the private-Telegram-owner channels + unknown-caller auto-registration rule,
  changelog row (1.19).
- README backlinks and ADR README index updated only if the index references
  ADR-0018 (check during apply).

## Non-goals

- Implementation of any bot/gateway code (the bot feature is its own change).
- Schema changes (CHECK already matches decision 1).
- Retiring Telegram entirely; leaving it as the supported alternative.

## Verification

- Structural readback of every file (dry-run echo, diff, then final).
- Cross-check: no contradicting lines left in PRD §3.8.9 and §7.
- `git diff --stat` sane before the commit.

## Tasks

- [x] T1 — Write ADR-0018 (status, date 2026-09-27, context, 3 decisions + 2 rules,
      consequences, related ADRs). Applied from writer draft; docs/architecture/README.md
      index row added by the parent.
- [x] T2 — PRD edits: §3.8.9 table + override warning removed; insert §3.8.10
      summary; §7 WhatsApp backlog item extended with the owner bot + auto-register
      rule + single-channel PIN; add changelog row (1.19). DONE via second writer
      delegation (+25/-3, tracking-first DIY edits verified).
- [x] T3 — Structural readback PASS → owner confirmed ("Dale") → pipeline
      (fmt/vet/build/test -race/lint all clean) → GGA hook passed (no staged
      code files — docs-only, expected) → committed a3925df on main, pushed
      (55d3c42..a3925df). Merge identity recorded here per ODD rule.

## Feature closing

**Status**: COMPLETE (2026-09-27). Commit `a3925df` on `main`, pushed to origin.
Next: the bot feature spec (per-sender + private owner bot + auto-register +
F-4 prerequisite) is the consumer of this ADR.

## Ownership

Owner: orchestrator (interactive) — delegated writing to `gentle-ai-worker` (T1/T2),
which returned persisted diff WITHOUT applying; orchestrator applied and verified.

# Proposal: feat-whatsapp-bot — Business-channel self-service registration, location contract, and Telegram owner-bot config

**Change:** feat-whatsapp-bot
**Status:** proposed (spec phase)
**Inputs:** `odd/tasks/feat-whatsapp-bot.md` (D1–D4 frozen), ADR-0018, ADR-0011, ADR-0017, PRD §3.8.9 / §3.8.10 / §7 Fase N
**Other artifacts:** `exploration.md`, `design.md`, `specs/**`, `tasks.md`

---

## Executive Summary

ADR-0018 freezes the channel model: clients, professionals and the owner write to **one** business number; Hermes's gateway reads each message's `from` and injects `X-Caller-Id`; the MCP server resolves the role from `accounts`/`clients` only. Today that model has two holes and one missing contract:

1. an unknown phone gets a **hard 401** with no self-service path — the auto-registration requirement ADR-0018 declares is unimplemented, and **no** `get_or_create_client` use case exists;
2. the location contract is one-directional: the owner can load coordinates, but `get_business_profile` returns no ready-to-send link and `update_business_profile` accepts no `geo:` input;
3. the owner-only private Telegram bot (ADR-0018 Decision 3b) has **no configuration surface**: the token, the chat allowlist and the owner phone used for the static `X-Caller-Id` injection must be written somewhere, by hand, today.

This change closes all three inside the existing architecture: a `register_client` MCP tool reachable through a narrow anonymous allowlist seam, backed by a new `get_or_create_client` use case that creates rows with `id == phone`; a derived `maps_url` plus `geo:` URI input; and a TUI option 7 extension that merges the Telegram owner-bot fields into `~/.hermes/config.yaml`. **No Go gateway/bridge code is written in this repository** — that is Hermes-side per ADR-0018.

## Business Problem

- A first-time customer messaging the business number is rejected by the server. Every new customer requires a manual DB step or a TUI-assisted insert, which no operator can do mid-conversation. The product promise of "self-service bookings through WhatsApp" is blocked at the first message.
- The trap is invisible: the closest existing repository helper (`ClientsRepo.GetOrCreate`) generates a **UUID** id, and the resolver matches `clients.id == phone`, so a naive "just reuse GetOrCreate" implementation would insert rows that are permanently unresolvable (silent data loss, no error).
- The owner cannot send a location to a customer from coordinates without the bot doing arithmetic, and cannot paste a `geo:` link into chat to correct a pin.
- The owner-only Telegram bot fields are undefined: without a written, validated, merge-preserving config path the operator hand-edits YAML (secret handling, YAML 1.1 `+54…` integer trap, unrelated keys at risk).

## Product Outcome

- A first-time phone that writes to the business channel becomes a `client` on its own: Hermes asks for a name (optional), calls `register_client`, retries the original tool call, and the booking flow proceeds. No operator involvement.
- `get_business_profile` returns `maps_url` whenever both coordinates are set, so the bot forwards a link with zero arithmetic; `update_business_profile` accepts `geo:lat,long` as an alternative to numeric `latitude`/`longitude`.
- The TUI option 7 flow writes and validates the Telegram owner-bot configuration (bot token, allowlist chat id, owner phone) into `~/.hermes/config.yaml` with merge preservation, atomic write and the existing quoting convention — so the owner never hand-edits YAML.
- Spam cannot become a database-growth vector: registration is rate-limited per phone with a configurable, conservative default and a semantic Spanish error.

## Scope

1. **New MCP tool `register_client`** (`internal/mcp`): typed input DTO with an **optional** display name only; the phone is taken from `X-Caller-Id`, never from the body; output `{client_id, display_name, created}`.
2. **New anonymous allowlist seam** (`internal/auth/middleware.go`): a static, code-owned allowlist evaluated **before** caller resolution; only the registration path is listed. No role is ever derived from config or from the seam.
3. **New use case `get_or_create_client`** (`internal/application/usecase`): phone validation, accounts collision rejection, placeholder name, registration rate limit, audit event.
4. **New domain port method** for phone-keyed creation plus its SQLite adapter (`internal/repository/clients.go`) using prepared statements and an `id == phone` insert inside a transaction.
5. **RBAC + DI wiring** (`cmd/mcp-server/main.go`): the new tool registered as anonymous-allowed in the permission map and in the mux; `ClientsRepo.GetOrCreate`'s UUID path explicitly rejected for this flow.
6. **Derived `maps_url`** in the `get_business_profile` wire output (transport layer only).
7. **`geo:` URI alternative input** for `update_business_profile` (RFC 5870, `geo:lat,long`), parsed in the transport adapter so only normalized numbers reach the use case (F-4 layering precedent).
8. **TUI option 7 extension**: new Telegram owner-bot field group (bot token, allowlist chat id, owner phone), validated and merged into the existing `mcp_servers.mcp-appointments` document without touching foreign keys; atomic write, `0600`, secrets never echoed.
9. **Gateway contract documentation**: per-sender `X-Caller-Id` injection, the `401 → ask name (optional) → register_client → retry` loop, `request_contact` for the Telegram business channel, and scenarios A/B including self-chat.

## Non-goals

- **No Go gateway or bridge in this repository.** Messenger I/O, `from` extraction, `request_contact` handling and per-sender header injection are Hermes-side per ADR-0018; this repo only documents the contract and serves anonymous registration.
- **No `business_profile` schema migration.** `messenger_platform`/`messenger_id` and their CHECK are unchanged (ADR-0018 Decision 1).
- **No Transfer Ownership and no `messenger_platform` data transition.** Moving the demo owner phone to the real one and flipping the platform to `whatsapp` are operator steps through the existing TUI/tools; they are documented, not automated.
- **No enrichment beyond the optional display name.** No email, no birthdate, no preferences, no address, no phone normalization/canonicalization.
- **No new capability for clients other than registration**: no MCP tool to list, edit or delete clients (that remains TUI/ADR-0016 territory).
- **No change to the resolver's read-only contract**: resolution never writes.
- **No Telegram owner-bot runtime in this repo** (no polling, no webhook, no allowlist enforcement code — enforcement is the gateway's).

## Affected Areas

| Area | Files | Change |
|------|-------|--------|
| Domain ports | `internal/domain/repository` | New phone-keyed get-or-create method on the client repository port |
| Use case | `internal/application/usecase/get_or_create_client.go` (new) | Validation, accounts collision, placeholder name, rate limit, audit |
| Persistence | `internal/repository/clients.go` | `id == phone` insert, prepared statements, `INSERT OR IGNORE` + read in one tx |
| Transport | `internal/mcp/tools_client_registration.go` (new), `internal/mcp/errors.go` (reuse), `internal/mcp/tools_profile.go` | New handler/DTO; `maps_url`; `geo:` parsing |
| Auth | `internal/auth/middleware.go` | Anonymous allowlist evaluated before resolution |
| Composition root | `cmd/mcp-server/main.go` | RBAC map entry (anonymous) + mux + DI |
| Config | `internal/config` | Rate-limit setting; Telegram owner-bot field names |
| TUI | `internal/tui` | Option 7 extension (field group, validation, merge write) |
| Docs | `docs/**`, `openspec/specs/**` | Gateway contract, operator steps; capability deltas are listed in the next row |
| Capability deltas | `specs/client-registration/spec.md` (NEW capability), `specs/hermes-config-tui/spec.md` (NEW capability), `specs/auth-middleware/spec.md` (MODIFIED capability delta — one MODIFIED + ADDED requirements), `specs/business-profile/spec.md` (MODIFIED capability delta — ADDED requirements), `specs/mcp-transport/spec.md` (**MODIFIED** delta — `tools/list` 19→20, `maps_url`, `location_uri`), `specs/clients/spec.md` (**ADDED** delta — `GetOrCreateByPhone`) | Requirements and traceability for the new contracts |
| Tests | alongside each file | unit per layer + integration through the real mux |

## Decision Log

| # | Decision (from D1–D4) | Expansion and rationale |
|---|-----------------------|-------------------------|
| D1 | **Scope = server-side contract + Telegram owner-bot config + TUI extension; no Go gateway here.** | ADR-0018 places the gateway on the messaging-platform side (Hermes). Adding a Go bridge here would create a second, untested identity-injection path and duplicate Hermes's transport work. The server-side contract plus documented gateway protocol is the smallest complete slice that lets Hermes implement the bot without further server changes. |
| D2 | **New tool `register_client`, anonymous-allowed through a seam before caller resolution; the tool registers the caller itself.** | The server cannot identify a first-time phone without a header-injected identifier, and it must not create rows on unauthenticated tool calls. The seam is the narrowest possible hole: a static code-owned allowlist, evaluated before resolution, admitting the registration path only. The phone MUST come from `X-Caller-Id` (already gateway-controlled, same trust point as every other identity); accepting it in the body would let the LLM register arbitrary third-party numbers. Creation uses `id == phone` because the resolver matches `clients.id == phone`; `ClientsRepo.GetOrCreate`'s UUID path is explicitly rejected. The resolver stays read-only so that reads never write. |
| D2a | **Phones already present in `accounts` are rejected.** | `accounts` holds roles (owner/admin/staff). Auto-registering an owner/admin phone as a client row would create a shadow identity and, worse, could flip a former staff phone into a client role. Rejection is a semantic Spanish error telling the person to contact the business. |
| D3 | **Name optional → placeholder `Cliente {phone}`; rate limit configurable, conservative default (~10/hour), semantic error on exhaustion; audit logging.** | Friction kills conversion: asking for a name must never block registration, so the field is optional and the placeholder keeps every row human-readable. Anti-spam is bounded per phone so a single abusive sender cannot grow the table; the limit is configurable because legitimate bursts exist (reinstall, phone change). Audit entries record the event without leaking full phone numbers. |
| D4 | **Derived `maps_url` in `get_business_profile` output; `geo:` URI accepted by `update_business_profile`.** | The bot must forward a location link without doing arithmetic or knowing the URL scheme — a server-derived field is the only place this can be pinned once. The `geo:` URI is a standard (RFC 5870), trivially parseable, and the natural agent-facing input; parsing belongs to the transport adapter so the use case keeps receiving normalized numbers (the F-4 precedent: transport concerns stay in transport). Both directions ship together because the contract is only half useful otherwise. |

## Risks & Rollback

| # | Risk | Mitigation |
|---|------|------------|
| R1 | The anonymous allowlist becomes a bypass for other tools | Allowlist is a static code-owned map with exactly one entry; a test asserts every other path still requires resolution; no config or header can extend it |
| R2 | The UUID trap returns via `ClientsRepo.GetOrCreate` reuse | Spec pins `id == phone`; integration test posts an unknown phone via `X-Caller-Id`, then calls a client-role tool with the same header and asserts resolution succeeds |
| R3 | Registration spam / table growth | Per-phone rate limit, configurable, default ~10/hour; semantic Spanish error on exhaustion; audit event per rejection; no global bypass |
| R4 | Rate limiter is in-memory → resets on restart | Accepted: single-process loopback server, restarts are operator-initiated and rare; the DB unique constraint on `clients.phone` remains the hard backstop |
| R5 | LLM registers a phone it does not own | Phone comes from the gateway-controlled header; the LLM has no field to set it; a body field named phone is ignored and rejected by strict decoding |
| R6 | `geo:` parsing divergence (extra parameters, uncertainty `u=`) | Parser accepts exactly `geo:` + two numbers, validates `lat ∈ [-90,90]` / `long ∈ [-180,180]`, rejects everything else with a semantic error; tests pin each rejection |
| R7 | Both `geo:` and numeric coordinates sent at once | Semantic Spanish error, no partial write; explicit clear flags remain the only way to unload a coordinate (F-4 semantics untouched) |
| R8 | TUI write corrupts `~/.hermes/config.yaml` (foreign keys, YAML 1.1 `+54…`) | Node-level merge preserving foreign keys, temp file + atomic rename, `0600`, phone always quoted, existing option 7 conventions reused |
| R9 | Secret leakage (Telegram bot token, owner phone) | Token never echoed to TUI output or logs; audit logs mask the phone; file `0600`; failure messages name the field, not the value |
| R10 | Review budget: this is a multi-surface change | Work units capped at ~100 changed lines (tasks.md forecast); chained PR slice strategy |

**Rollback:** every piece is additive and independently revertible. Removing the allowlist entry + tool wiring restores the current 401 behavior for unknown phones (the clients created meanwhile stay valid and resolvable). Removing the `maps_url`/`geo:` handling returns the previous wire shape (additive field, and `location_uri` simply stops being accepted). Removing the TUI extension leaves a valid config document — the merge never deletes foreign keys. **No schema migration, no data transformation, no destructive step.**

## Success Criteria

- [ ] Unknown phone + `register_client` → client row with `id == phone`; the same `X-Caller-Id` then resolves as `role = client` on a normal tool call.
- [ ] `register_client` with a display name stores it; without one stores `Cliente {phone}`.
- [ ] Phone already in `accounts` → semantic Spanish error, no row created.
- [ ] A header phone longer than 15 digits is rejected on the registration path with a semantic Spanish error and no row is created (a 15-digit phone still registers).
- [ ] Exhausting the rate limit → semantic Spanish error; no row created; audit event emitted; limit value comes from server config with default ~10/hour.
- [ ] `register_client` is the only path reachable without a resolved caller; every other tool still returns 401 for unknown phones (test over the real mux).
- [ ] `get_business_profile` returns `maps_url = https://maps.google.com/?q=<lat>,<long>` when both coordinates are set, omits it otherwise.
- [ ] `update_business_profile` accepts `geo:lat,long` and stores the same values as numeric input; invalid/out-of-range/mixed payloads fail with semantic Spanish errors.
- [ ] TUI option 7 extension writes and validates bot token / allowlist chat id / owner phone, preserving foreign keys, with the owner phone quoted.
- [ ] Gateway contract documented (per-sender header injection, 401 → register → retry, `request_contact`, scenarios A/B + self-chat).
- [ ] `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...` clean; `go build -o /dev/null ./...` passes; `go test -v -race ./...` passes.

## Next

- `design.md` — flows, package layout, rate-limiter semantics, TUI merge, testing and security review.
- `specs/client-registration/spec.md` (NEW), `specs/auth-middleware/spec.md` (MODIFIED), `specs/business-profile/spec.md` (MODIFIED), `specs/hermes-config-tui/spec.md` (NEW), `specs/mcp-transport/spec.md` (MODIFIED delta — `tools/list` 19→20, `maps_url`, `location_uri`), `specs/clients/spec.md` (ADDED delta — `GetOrCreateByPhone`).
- `tasks.md` — phased work units ≤ ~100 lines with a Review Workload Forecast.

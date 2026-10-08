# Exploration: feat-whatsapp-bot

**Change:** feat-whatsapp-bot
**Source of truth:** `odd/tasks/feat-whatsapp-bot.md` (product decisions D1–D4 frozen 2026-10-08)
**Frozen upstream:** ADR-0018 (`docs/architecture/0018-communication-channels.md`), ADR-0011, ADR-0017, PRD §3.8.9 / §3.8.10 / §7 Fase N (changelog 1.20)
**Scope note:** all evidence below was verified by scout on 2026-10-08 and is cited as-is; the spec phase re-verifies nothing.
> **Amendment (readback fix pass):** the line citations in §1 and §3 and the `HasValidPhone` semantics in §5 were corrected after a structural readback; the comment/code gap found there is recorded inline.

---

## 1. The 401 chain (what happens today to an unknown phone)

| Step | Location | Fact |
|------|----------|------|
| Header read | `internal/auth/middleware.go:76` | `X-Caller-Id` is read **only** there (single extraction point). |
| Resolution | `internal/auth/resolver.go:105` | Unknown id → `authError{msgNotRecognized, ErrUnauthenticated}` with message `"no te reconozco. Por favor regístrate primero."` |
| Rejection | `internal/auth/middleware.go:83-90` | Resolver error → HTTP `401`, downstream handler never runs. |
| Wire translation | `internal/mcp/auth_translator.go:99-110` | 401 → JSON-RPC `-32000` (not `-32603`), so the LLM sees an auth-class error. |
| Path→tool bridge | `internal/mcp/auth_translator.go:73-74` | The translator maps the HTTP path to the MCP tool name to build the semantic message. |

**Consequence:** a first-time phone on the business channel gets a hard 401 with no self-service path. This is exactly the gap ADR-0018 declares as a requirement of this change.

## 2. No `get_or_create_client` exists — and the UUID trap

| Candidate | Location | Behavior | Verdict |
|-----------|----------|----------|---------|
| `ClientsRepo.GetOrCreate` | `internal/repository/clients.go:223` | Inserts a client with a generated **UUID** id | **Rejected for this flow**: `CallerResolver` matches `accounts.id` then `clients.id`, so a UUID-id client can never resolve from a phone (`auth-middleware` Requirement "Resolución del caller en 1-2 queries", steps 4–5). |
| `admin.AddSelfAsClient` | `internal/admin/clients.go:84-152` | Creates the client with `id == phone` deliberately | **Reference implementation**: proves the `id == phone` contract and the eventual shape of the row. |
| `get_or_create_client` (use case) | — | Does not exist | **To be created** by this change. |

**Trap to encode as a spec requirement:** creation MUST set `clients.id = phone`; the `clients.phone` column is populated with the same value (unique constraint is the second guard). Any path that generates a UUID id is out of contract for auto-registration.

## 3. RBAC and error-sanitization map (where new wiring lands)

| Concern | Location | Fact |
|---------|----------|------|
| Permission map | `cmd/mcp-server/main.go:396-414` | Per-tool RBAC map at the composition root; double enforcement with in-usecase `auth.RequireRole`. |
| Sanitization | `internal/mcp/errors.go:41-48` | Only `*domain.SemanticError` messages reach the client; everything else collapses to generic `-32603`. |
| Anonymous seam | — | None exists today: every MCP tool requires a resolved `Caller`. The seam must be added **before** caller resolution. |

## 4. Schema facts relevant to this change

- `business_profile.messenger_platform` CHECK `IN ('whatsapp','telegram')` and `messenger_id`: `internal/db/schema.go:19-33`. **No schema migration is needed** for the channel model (ADR-0018 Decision 1).
- `business_profile.latitude` / `longitude`: `REAL`, nullable, **no range CHECK**.
- `get_business_profile` wire output carries `lat`/`long` (`internal/mcp/tools_profile.go:25-49`) and **no** `maps_url`.
- `clients.phone` is `UNIQUE` (`openspec/specs/clients/spec.md` Requirement "`phone` is unique"); `clients` has **no** messenger columns by design.

## 5. Input validation state (friction for D2/D3/D4)

- `entity.Client.HasValidPhone` (`internal/domain/entity/client.go:23-40`): optional leading `+`, **minimum** 4 digits (5 with the `+`), digits only, and **no upper bound** — the doc comment says "4–15 digits (E.164 subset)" (`internal/domain/entity/client.go:21-22`) but the code enforces no maximum. **Recorded gap**: comment vs. code (the 15-digit E.164 cap is added by this change on the registration path only, see `specs/client-registration/spec.md`). **No normalization** (no E.164 canonicalization, no separator stripping).
- `internal/validation` is a **doc-only stub** — no reusable validators to lean on.
- **Consequence:** the phone used as `clients.id` is whatever the gateway injected in `X-Caller-Id`. Auto-registration MUST validate (`HasValidPhone`) and MUST NOT silently normalize, or the row id will not match the next incoming header.

## 6. D4 prerequisite — RESOLVED

F-4 clear-null shipped in merge `3b86db9`:

- Custom `UnmarshalJSON` with unexported presence markers: `internal/mcp/tools_maintenance.go:106`.
- `ClearLatitude` / `ClearLongitude` flags reach the DTO.
- `applyProfileReferenceUpdates` honors the clear flags: `internal/application/usecase/update_business_profile.go:161`.

So `update_business_profile` can already unload a bad pin; the `geo:` URI input (D4) and the derived `maps_url` are additive on top.

## 7. Layer map for the new work (Clean/Hexagonal, ADR-0013)

```
domain port (new method)  internal/domain/repository
      ▲
use case (new)            internal/application/usecase/get_or_create_client.go
      ▲
transport DTO + tool      internal/mcp/   (wire tags only here; geo: parsing here)
      ▲
wiring / RBAC / allowlist cmd/mcp-server/main.go, internal/auth/middleware.go
      ▼
SQL adapter (prepared)    internal/repository/clients.go
```

## 8. Open questions carried to design (not product decisions)

1. Wire field name for the `geo:` alternative input (assumption: `location_uri`).
2. Config key names for registration rate limit and Telegram owner-bot fields (must follow existing `MCP_*` env conventions).
3. Audit-log PII policy: existing `slog` audit records raw caller ids; this change pins **masked** phone in new registration audit events (follow-up: align existing events).

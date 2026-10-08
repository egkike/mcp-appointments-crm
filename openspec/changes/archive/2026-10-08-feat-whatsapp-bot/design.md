# Design: feat-whatsapp-bot

**Change:** feat-whatsapp-bot
**Inputs:** `proposal.md` (D1–D4), `exploration.md`, ADR-0018, ADR-0013 (layering), ADR-0017 (TUI option 7), F-4 precedent (`3b86db9`)
**Constraint:** no new third-party dependency. Everything below is stdlib + already-present deps (`gopkg.in/yaml.v3` for the TUI merge, per ADR-0017).

---

## 0. Layering contract (non-negotiable, ADR-0013)

| Layer | Location | This change adds |
|-------|----------|------------------|
| Domain | `internal/domain/repository` | One port method for phone-keyed get-or-create (no SQL, no wire tags) |
| Application | `internal/application/usecase/get_or_create_client.go` | Orchestration: validation, accounts collision, placeholder, rate limit, audit |
| Transport | `internal/mcp/tools_client_registration.go`, `internal/mcp/tools_profile.go` | Typed input DTO (JSON tags), `maps_url`, `geo:` parsing |
| Auth edge | `internal/auth/middleware.go` | Static anonymous allowlist evaluated **before** resolution |
| Persistence | `internal/repository/clients.go` | `id == phone` insert, prepared statements, one transaction |
| Composition root | `cmd/mcp-server/main.go` | RBAC map entry (anonymous) + mux + DI |
| UI adapter | `internal/tui` | Option 7 extension (Telegram owner-bot fields) |
| Config | `internal/config` | Rate-limit setting; field names for owner-bot values |

Wire JSON tags exist only in `internal/mcp`. `geo:` parsing exists only in `internal/mcp`. SQL exists only in `internal/repository`. RBAC is enforced twice (composition-root map + in-usecase check).

---

## 1. `register_client` — anonymous path (D2)

### 1.1 Middleware seam: allowlist before resolution

```
HTTP request
  │
  ├─ extract X-Caller-Id (existing, internal/auth/middleware.go:76)
  │
  ├─ NEW step 0: header absent/empty ? ── yes ─► HTTP 401 "no se proporcionó X-Caller-Id" (universal, runs for EVERY path incl. allowlisted ones)
  │
  ├─ NEW: isAnonymousAllowed(r.Path) ?  ── yes ─► attach AnonymousCaller marker to ctx, skip resolution
  │                                     └─ no ──► existing resolver path (unchanged, read-only)
  │
  └─ downstream handler (RBAC map check happens at the composition root)
```

Rules the seam must obey:

- The allowlist is a **static, code-owned** map (`map[string]struct{}` keyed by MCP path) with exactly one entry: the `register_client` path. It is not configurable, not header-driven, not extensible at runtime.
- The seam runs **before** caller resolution, and **after** the universal empty-header check (which answers `401` for a missing/empty header on every path, allowlisted or not). It must not touch the database.
- The seam marks the context as anonymous; it never fabricates a `Caller` with a role. The use case receives an explicit `CallerID` string sourced from the header and **no role**.
- Every non-allowlisted path keeps the exact current behavior (401 → `-32000` via `auth_translator.go`).
- Registration with a **missing** (or empty) `X-Caller-Id` never reaches the seam: the universal empty-header check answers HTTP `401` (`"no se proporcionó X-Caller-Id"`, translated to `-32000` via `auth_translator.go`), exactly as for every other path — the allowlist cannot wave it away (`auth-middleware` precedence).

### 1.2 Tool shape (transport)

| Wire field | Type | Required | Notes |
|---|---|---|---|
| `display_name` | `*string` | no | Optional display name; trimmed; max 80 runes; empty string treated as absent |
| — | — | — | **No phone field.** Strict decoding: an unknown `phone`/`caller_id` field is a semantic error, not a silent ignore |

Output: `{ "client_id": "<phone>", "display_name": "<stored name>", "created": true|false }` (`created=false` when the row already existed).

Errors (all `*domain.SemanticError`, so `internal/mcp/errors.go:41-48` passes them through):

| Condition | Spanish message (example) |
|---|---|
| missing/empty header | not a semantic error: the universal empty-header check answers HTTP `401` `"no se proporcionó X-Caller-Id"` (JSON-RPC `-32000`) before the seam — see §1.1 |
| invalid phone | `Error: el teléfono no tiene un formato válido (se esperan entre 4 y 15 dígitos, con + opcional).` |
| phone exists in `accounts` | `Error: este número ya pertenece a una cuenta del negocio. Contactá al administrador.` |
| rate limit exhausted | `Error: alcanzaste el límite de registros automáticos. Probá de nuevo más tarde o pedile al negocio que te registre.` |
| unknown body field | `Error: el campo '<name>' no está permitido.` |

### 1.3 Use case `get_or_create_client`

Order of operations (fail closed, no writes before all checks pass):

1. `callerID` non-empty, at most 15 digits (E.164 cap; registration path only — `HasValidPhone` itself has no upper bound, §8), and `entity.Client.HasValidPhone(callerID)` → else invalid-phone error.
2. **Accounts collision check** (read-only, via the accounts port): if a row exists in `accounts` for this phone → collision error + audit event `registration_rejected_account`. Covers active *and* inactive accounts.
3. **Rate limit check** (section 1.5) keyed by phone → else rate-limit error + audit event `registration_rate_limited`.
4. Compute stored name: `displayName` if non-empty after trim, else `fmt.Sprintf("Cliente %s", callerID)`.
5. Call the new port method → `(client, created, err)`.
6. Audit events `client_registered` (or `client_registration_noop` when `created == false`) with the **masked** phone; the rejection paths audit `registration_rejected_account` (step 2) and `registration_rate_limited` (step 3), so every outcome is audited.
7. Return DTO `{ClientID, DisplayName, Created}`.

No `auth.RequireRole` here: this use case is the one place where the identity is a bare header value with no role. Instead it MUST NOT accept a role parameter (compile-time guarantee) and MUST NOT use `GetOrCreate`'s UUID path.

### 1.4 Port method and SQL adapter

Port (new method on the existing client repository interface):

```go
// GetOrCreateByPhone returns the client whose id equals phone, creating it when absent.
// The returned bool reports whether a row was created by this call.
GetOrCreateByPhone(ctx context.Context, phone, displayName string) (entity.Client, bool, error)
```

Adapter (`internal/repository/clients.go`), race-safe under WAL concurrency:

```
BEGIN IMMEDIATE
  INSERT OR IGNORE INTO clients (id, name, phone) VALUES (?, ?, ?)   -- id == phone, phone == phone
  SELECT id, name, phone FROM clients WHERE id = ?                   -- read back authoritative row
COMMIT
```

- `id == phone` is the contract: the resolver matches `clients.id == phone`. `clients.phone` carries the same value (`UNIQUE` is the second guard).
- `created` is derived from `RowsAffected()` of the `INSERT OR IGNORE`, not from a prior SELECT.
- All values bound with `?`; no concatenation; `context.Context` propagated; transaction rolled back on any error.
- **Explicitly rejected:** reusing `ClientsRepo.GetOrCreate` (`internal/repository/clients.go:223`) — its generated UUID id can never resolve (see `exploration.md` §2).

**Auth-free registration reads.** Registration runs before any `Caller` exists, so `AccountsRepo.FindByID` (authenticated) and `ClientsRepo.FindByPhone` (admin/owner) fail at runtime. A narrow port `RegistrationLookup` (`AccountExistsByID`, `FindClientByPhoneAny`) carries only those two reads, delegates to the same tables, and is wired ONLY into `get_or_create_client`; no existing guard is weakened. Same chicken-and-egg escape as the TUI's `ClientsSelfService`. Phase 3 integration must assert the anonymous path end-to-end through the real mux.

### 1.5 Rate limiter

| Aspect | Decision |
|---|---|
| Key | phone (the header value), not IP — the server sees loopback traffic only |
| Algorithm | fixed 1-hour window counter, `sync.Mutex`-guarded `map[string]int` + window start timestamp |
| Limit source | server config, env-overridable, existing `MCP_*` convention; **default 10/hour**. **Assumption (proposed — confirm at apply):** the exact config shape (field/struct) and env key spelling |
| `0` semantics | disables auto-registration entirely (fail closed), returns the rate-limit error |
| State | in-memory, single process; reset on restart is accepted (R4) |
| Rejections counted? | Yes — an exhausted phone stays rejected for the rest of the window (no bypass by refreshing) |
| Placement | inside the use case so any future transport inherits it |
| Clock | injected `func() time.Time` for deterministic tests |

### 1.6 Gateway flow (documented contract, no Go here)

```
inbound message → gateway reads `from` → injects X-Caller-Id (per sender)
  → tool call
     ├─ success → proceed
     └─ 401 / -32000 "no te reconozco"
          → ask the sender for a display name (OPTIONAL, must not block)
          → register_client {display_name?}          (header already carries the phone)
          → retry the original tool call once
          → if the retry still fails → report to the owner, do not loop
```

Telegram business channel adds one step before the first tool call: `request_contact` (Telegram does not expose the sender's phone to bots, so the phone must be collected in-chat and then used as `X-Caller-Id`). Outreach outside the allowlist is rejected **in the gateway**, before any tool call reaches the server.

Scenario coverage the docs must state explicitly (PRD §7 Fase N):

| Scenario | Sender | Session number | Resolution |
|---|---|---|---|
| A | owner from a personal number that differs from the business number | business number | incoming JID resolves as owner via `accounts` naturally |
| B | owner whose number **is** the business number, using WhatsApp self-chat | same as owner account | self-chat sender MUST be treated as a valid sender resolving via `accounts` |
| Client | any unknown phone | business number | 401 → `register_client` → retry |

---

## 2. Derived `maps_url` (D4)

- Where: `internal/mcp/tools_profile.go` (`get_business_profile` handler) — a **wire-only** derivation, no domain/repo change.
- Rule: emit `maps_url = "https://maps.google.com/?q=" + fmtLat + "," + fmtLong` **only** when both `latitude` and `longitude` are non-null; otherwise the key is absent.
- Formatting: `strconv.FormatFloat(v, 'f', -1, 64)` so no scientific notation can leak into a URL; `nil` never becomes `"0"`.
- `lat`/`long` remain in the output unchanged (additive change, backward compatible).

---

## 3. `geo:` URI input for `update_business_profile` (D4)

- New optional wire field: **`location_uri`** (`*string`). **Assumption (proposed — confirm at apply):** the field name itself; it is chosen to avoid colliding with `latitude`/`longitude` and is confirmed by the owner's docs review at apply time.
- Accepted grammar (RFC 5870, strict subset): `geo:` `lat` `,` `long` — optional leading `+`/`-`, decimal with `.`; no whitespace; no `;u=` uncertainty parameter (rejected, not ignored).
- Validation: `lat ∈ [-90, 90]`, `long ∈ [-180, 180]`; parse via `strconv.ParseFloat` with error discarded in favor of a semantic message.
- Mapping: `location_uri` fills the existing numeric `latitude`/`longitude` DTO fields **before** the use case is called. The use case keeps receiving normalized numbers only (F-4 layering precedent: `internal/mcp/tools_maintenance.go:106`).
- Conflicts and interactions:
  - `location_uri` + numeric `latitude`/`longitude` in the same payload → semantic error `"Error: no puedo combinar una ubicación geo: con latitude/longitude numéricas."`, no partial write.
  - `location_uri` together with F-4 clear flags → semantic error (clear + set is contradictory).
  - `location_uri` alone behaves exactly like sending `latitude`/`longitude`.
- Error message example: `"Error: la ubicación no tiene un formato válido. Usá geo:lat,long (por ejemplo geo:-34.6037,-58.3816)."`

---

## 4. Telegram owner-bot config + TUI option 7 extension (D1)

### 4.1 Fields written into `~/.hermes/config.yaml`

| Field | Kind | Validation | Notes |
|---|---|---|---|
| bot token | secret string | non-empty, matches Telegram bot-token shape `^\d{8,12}:[A-Za-z0-9_-]{30,40}$` | never echoed, never logged |
| allowlist chat id | integer id (string on the wire) | numeric, optional leading `-` (groups), 5–20 digits | the only chat allowed to talk to the owner bot |
| owner phone | E.164-ish string | `entity.Client.HasValidPhone` (optional `+`, ≥4 digits — the entity validator has no upper bound) plus a ≤15-digit cap enforced by the TUI validator (E.164 maximum) | source of truth for the static `X-Caller-Id` injection (ADR-0018 D3b) |

Placement: the existing `mcp_servers.mcp-appointments` entry's `env` block (same document option 7 already merges), so the values land where the server reads its configuration. Exact key spellings follow the repo's `MCP_*` convention and are **proposed — confirm at apply**: they are NOT pinned in this change's spec deltas (the `hermes-config-tui` delta pins the values and validations, not the key spellings); the implementation task reconciles them against `internal/config`.

### 4.2 Merge and write (same conventions as today's option 7)

1. Read `~/.hermes/config.yaml` with `yaml.v3` **node-level** decoding (`yaml.Node`) so foreign keys and comments-adjacent structures survive.
2. Locate `mcp_servers.mcp-appointments`; if absent, create only that subtree (existing behavior).
3. Set/replace the three fields; every phone-like value is emitted **quoted with `+`** (`"\u002B549…"`) because YAML 1.1 parses a bare `+54…` as an integer.
4. Write to a sibling temp file (`0600`), `fsync`, then `os.Rename` — atomic replace, no partial file.
5. On any error: the original document is untouched; the TUI shows a Spanish failure message naming the field.
6. Output summary masks the token and the phone (e.g. `+5491****1111`).

### 4.3 TUI wiring

- Option 7 gains a second step (field group) reachable from the existing flow; Bubble Tea model/update/view follow MVU with one field per component and per-field validation before advancing (project convention).
- Owner phone is **prefilled** from the active owner (`accounts`), reusing the current prefill behavior.
- Non-TTY (console) flow mirrors the same fields and validations, as option 7 already does today (ADR-0017).
- Cancel/back leaves the config file untouched.

---

## 5. Testing approach

| Layer | Test focus |
|---|---|
| Domain/ports | compile-level contract only (interface method signature) |
| Repository (`internal/repository`) | `id == phone` row shape; `created` true/false; concurrent double-registration yields one row; `clients.phone` populated; `INSERT OR IGNORE` path uses placeholders |
| Use case (`internal/application/usecase`) | valid/invalid phone (>15 digits rejected, 15 accepted); accounts-collision (active + inactive); placeholder name; name trimmed and length-capped; rate limit exhaustion with injected clock; `0` disables; audit events (`client_registered`, `client_registration_noop`, `registration_rejected_account`, `registration_rate_limited`) emitted with masked phone |
| Middleware (`internal/auth`) | allowlisted path skips resolution and reaches the handler with no caller; every other path still 401s; allowlist is not header- or config-extensible |
| Transport (`internal/mcp`) | `register_client` DTO strictness (unknown field rejected, phone in body rejected), output shape, semantic errors pass through `errors.go`; `maps_url` present/absent/format; `geo:` accepted, malformed/out-of-range/mixed rejected |
| Integration (real mux) | unknown phone → `register_client` → client row → same header resolves `role = client` on a client-role tool; rate limit over the mux; owner/admin phone rejected |
| TUI (`internal/tui`) | field validation table; merge preserves foreign keys; phone quoted; atomic write via `t.TempDir()`; token never in output |
| Docs | structural readback of the gateway contract doc (no executable test) |

Commands (per AGENTS.md, run at phase gates, not per work unit): `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`. Focused tests first per work unit.

---

## 6. Security review

| Surface | Control |
|---|---|
| Spam / table growth | per-phone fixed-window rate limit (default 10/hour, configurable, `0` = disabled/fail-closed), semantic error, audit event on rejection; DB `UNIQUE` on `clients.phone` as hard backstop |
| Privilege escalation | the seam grants **no role**; the use case never reads a role; a manipulated config can only choose an already-authorized identity (ADR-0018 Consecuencias); `accounts` phones are rejected so registration can never shadow a staff/owner identity |
| Identity spoofing | phone comes only from `X-Caller-Id`; no body field can set it; unknown body fields are rejected |
| SQL injection | prepared statements only, `?` placeholders in every new statement; identifiers are literals in code |
| Input validation | `HasValidPhone` for phones; regex for token/chat id; strict range checks for coordinates; strict numeric parsing for `geo:` |
| Error leakage | only `*domain.SemanticError` reaches the client (`internal/mcp/errors.go:41-48`); no SQL, no paths, no stack traces |
| Secret handling | token never echoed or logged; config file `0600`; masked phone in TUI output and audit logs |
| Loopback invariant | unchanged — the server still binds `127.0.0.1:3000`; the owner bot talks to it over loopback and gateway-side allowlist enforcement is documented, not weakened |
| Rate-limiter memory | bounded by distinct phone cardinality within the window; entries expire with the window (documented, not GC'd in v1) |

---

## 7. Rollout / rollback

**Rollout order:** (1) domain port + repository; (2) use case + rate limiter; (3) middleware seam + tool + wiring (this is the step that changes external behavior for unknown phones); (4) location contract (independent); (5) TUI option 7 extension (independent); (6) gateway contract docs.

**Rollback:** each step is independently revertible. Reverting step 3 restores today's 401 for unknown phones without touching data already created (rows remain valid and resolvable). Reverting step 4 removes an additive wire field and an additive input. Reverting step 5 leaves a valid config document. **No migration to reverse, no destructive operation in any step.**

**Operator steps documented, not automated** (non-goals): Transfer Ownership to the real owner phone, `messenger_platform` transition to `whatsapp`.

---

## 8. Open follow-ups (out of scope, recorded)

- Phone **normalization debt**: `HasValidPhone` does not canonicalize, so `+54 9 11 …` and `+54911…` are different identities. A normalization pass needs its own change (and a `clients.id` migration story). Related recorded gap (§5/§8 of `exploration.md`): the doc comment claims "4–15 digits" while the code enforces no upper bound; this change adds the ≤15-digit cap on the registration path (and in the TUI owner-phone validator) only, leaving the entity and every other flow untouched.
- `internal/validation` is a doc-only stub: the new validators live next to their use until that package earns real content.
- `geo:` URI input for other location-bearing tools (if any appear) should reuse the same parser.
- Audit-log PII alignment: existing `slog` audit events log raw caller ids; only the new registration events mask. Aligning the rest is a separate hardening change.
- Rate-limit persistence: in-memory state resets on restart (accepted R4); a persisted counter would need a table and is not justified yet.

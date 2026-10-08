# Tasks: feat-whatsapp-bot

> **Change:** feat-whatsapp-bot
> **Status:** tasks drafted (spec phase) — ready for apply planning
> **Inputs:** `proposal.md`, `design.md`, `specs/client-registration/spec.md`, `specs/auth-middleware/spec.md`, `specs/business-profile/spec.md`, `specs/hermes-config-tui/spec.md`, `specs/mcp-transport/spec.md` (MODIFIED delta), `specs/clients/spec.md` (ADDED delta)
> **Hard rule:** every work unit ≤ ~100 changed lines (review band). Test-first for all Go units.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1150–1450 (≈470 impl + ≈620 tests + ≈180 docs/spec) |
| 400-line budget risk | **High** |
| Chained PRs recommended | **Yes** |
| Suggested split | PR1 registration core (persistence → use case) → PR2 anonymous seam + tool + wiring → PR3 location contract → PR4 TUI option 7 → PR5 gateway docs |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main (each PR targets `main` from its own feature branch, merged in order) |

Each work unit below is sized to fit a ≤ ~100-line review slice; a unit that grows past that MUST be split before apply. `Decision needed before apply: Yes`. `Chained PRs recommended: Yes`. `Chain strategy: stacked-to-main`. `400-line budget risk: High`.

---

## Phase 1 — Registration persistence (PR1)

> Depends on nothing. Additive port method + adapter + tests.

### TASK-1.1 — RED: repository test for `GetOrCreateByPhone`

- [ ] Add a failing test in `internal/repository` covering: creation with `id == phone`, `clients.phone` populated, `created == true`
- [ ] Second case: existing row returns `created == false` and does not duplicate
- [ ] Third case: two concurrent calls yield exactly one row
- [ ] Record the observed failure output before implementing

### TASK-1.2 — GREEN: port method + SQLite adapter

- [ ] Add the port method `GetOrCreateByPhone(ctx, phone, displayName) (entity.Client, bool, error)` to the client repository interface in `internal/domain/repository` (doc comment pins the `id == phone` invariant and the UUID rejection)
- [ ] Implement it in `internal/repository/clients.go`: one `BEGIN IMMEDIATE` transaction, `INSERT OR IGNORE INTO clients (id, name, phone) VALUES (?, ?, ?)`, read-back `SELECT`, `created` from `RowsAffected()`
- [ ] Every value bound with `?`; `ctx` propagated; rollback on error; no UUID generation, no reuse of `GetOrCreate`
- [ ] Run TASK-1.1 tests green
- [ ] **Gate:** focused `go test -race ./internal/repository/...` + `go build -o /dev/null ./...`

---

## Phase 2 — Use case `get_or_create_client` (PR1 cont.)

> **Legacy-row edge (independent verifier, 2026-10-08):** a pre-existing `clients` row with `phone == p` but `id == UUID` (inserted by the legacy `GetOrCreate`) makes `INSERT OR IGNORE` a no-op and the id read-back miss → internal semantic error. TASK-2.2 must decide the semantics (read-by-phone fallback vs explicit conflict Spanish error) before shipping the use case; the adapter stays per REQ-CL-PORT-001.

### TASK-2.1 — RED: use-case tests (validation + collision + placeholder)

- [ ] Failing tests in `internal/application/usecase`: invalid phone rejected; **phone longer than 15 digits rejected and a 15-digit phone accepted (registration path only)**; phone present in `accounts` (active and inactive) rejected; placeholder `Cliente {phone}` stored when no name; name trimmed; name longer than 80 runes handled deterministically
- [ ] Assert every failure is a `*domain.SemanticError` with a Spanish message (including the >15-digit rejection); assert no write happens before checks pass
- [ ] Record the observed failure output

### TASK-2.2 — GREEN: use case implementation

- [ ] Create `internal/application/usecase/get_or_create_client.go` with the 7-step order from `design.md` §1.3
- [ ] Reject phones longer than 15 digits with the invalid-phone semantic error (the entity validator enforces no upper bound — registration path only)
- [ ] Signature takes an explicit caller-id string and **no role parameter**
- [ ] Reuse the accounts port read for the collision check (add a small read-only port method if absent — flag the addition in the PR body)
- [ ] Wire the new use case in `cmd/mcp-server/main.go` DI (behavior unchanged until the tool exists)
- [ ] Run TASK-2.1 green

### TASK-2.3 — RED: rate limiter tests

- [ ] Failing tests for a `registrationRateLimiter` (own file, injected clock `func() time.Time`): limit reached → rejection; per-phone isolation; window rollover resets the counter; `0` disables registration; rejection after exhaustion within the window
- [ ] Record the observed failure output

### TASK-2.4 — GREEN: rate limiter + config setting + audit events

- [ ] Implement the fixed-window limiter (mutex-guarded map, injected clock), placed **inside the use case** so all transports inherit it
- [ ] Add the configurable limit to `internal/config` (existing `MCP_*` env convention; default 10/hour; `0` = disabled/fail-closed) and document it in the PR body
- [ ] Emit audit events (`client_registered`, `client_registration_noop`, `registration_rejected_account`, `registration_rate_limited`) with a **masked** phone
- [ ] Run TASK-2.3 green
- [ ] **Gate:** `go test -race ./internal/application/... ./internal/repository/...` + `go build -o /dev/null ./...`

---

## Phase 3 — Anonymous seam + `register_client` tool + wiring (PR2)

> This phase changes external behavior for unknown phones. Order matters: seam first, then tool, then the RBAC map entry.

### TASK-3.1 — RED: middleware seam tests

- [ ] Failing tests in `internal/auth`: allowlisted path reaches the handler with **no** `accounts`/`clients` query and no role in context; every other path with an unknown id still answers `401`; a missing or empty header on the allowlisted path yields the universal `401` **before** the seam (same order as any other path); the allowlist has exactly one entry and cannot be extended (table test over the whole tool path surface)
- [ ] Record the observed failure output

### TASK-3.2 — GREEN: seam implementation

- [ ] Add the static code-owned allowlist + anonymous context marker in `internal/auth/middleware.go`, evaluated before resolution (`design.md` §1.1)
- [ ] No DB access, no config read, no header influence in the seam
- [ ] Run TASK-3.1 green

### TASK-3.3 — RED: transport tests for `register_client`

- [ ] Failing tests in `internal/mcp`: strict decoding rejects a body `phone`/unknown key; `display_name` optional; output shape `{client_id, display_name, created}`; each semantic error passes through `errors.go`
- [ ] Record the observed failure output

### TASK-3.4 — GREEN: handler, DTO, RBAC map entry and mux wiring

- [ ] Create `internal/mcp/tools_client_registration.go` (typed input DTO with JSON tags, `display_name *string` only) and the handler that calls the use case with the header phone
- [ ] Add the `register_client` entry to the RBAC map (`cmd/mcp-server/main.go:396-414`) as anonymous-allowed and register it in the mux
- [ ] Run TASK-3.3 green

### TASK-3.5 — Integration test through the real mux

- [ ] Add the end-to-end test: unknown phone → `register_client` → row created with `id == phone` → same header resolves `role = client` on a client-role tool
- [ ] Add the negative case: owner/admin phone rejected; rate limit exhausted over the mux
- [ ] **Gate:** `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`

---

## Phase 4 — Location contract (PR3)

### TASK-4.1 — RED: `maps_url` tests

- [ ] Failing tests: both coordinates → exact URL; either missing → key absent; tiny value → no scientific notation; existing `lat`/`long` unchanged
- [ ] Record the observed failure output

### TASK-4.2 — GREEN: `maps_url` in the wire output

- [ ] Add the derived field in `internal/mcp/tools_profile.go` only (no domain/repo change), with `strconv.FormatFloat(v, 'f', -1, 64)`
- [ ] Run TASK-4.1 green

### TASK-4.3 — RED: `geo:` parsing tests

- [ ] Failing table tests: valid `geo:lat,long`; optional signs; out-of-range lat/long; `;u=` parameter; whitespace; non-numeric; mixed with numeric coordinates; mixed with clear flags
- [ ] Record the observed failure output

### TASK-4.4 — GREEN: `location_uri` parsing in the transport adapter

- [ ] Add `location_uri *string` to the `update_business_profile` input DTO and parse it before the use case call, populating the numeric fields (F-4 precedent)
- [ ] Add the conflict/range semantic Spanish errors; use case and repository untouched
- [ ] Run TASK-4.3 green
- [ ] **Gate:** focused `go test -race ./internal/mcp/...` then the full pipeline

---

## Phase 5 — TUI option 7 extension (PR4)

### TASK-5.1 — RED: validation + merge tests

- [ ] Failing tests in `internal/tui`: token/chat-id/phone validation table; merge preserves foreign keys; phone written quoted; atomic write against `t.TempDir()`; token absent from output; cancel leaves the file byte-identical
- [ ] Record the observed failure output

### TASK-5.2 — GREEN: field model + validators

- [ ] Add the Telegram owner-bot field group to the option 7 model with per-field validation (MVU; one component per field) and owner-phone prefill from `accounts`
- [ ] Non-TTY console parity for the same fields and validators

### TASK-5.3 — GREEN: node-level merge + atomic write

- [ ] Extend the existing option 7 writer: `yaml.Node` merge into `mcp_servers.mcp-appointments`, temp sibling file `0600` + flush + `os.Rename`, masked summary output
- [ ] Run TASK-5.1 green
- [ ] **Gate:** `go test -race ./internal/tui/...` + the full pipeline

---

## Phase 6 — Gateway contract docs + repo cross-references (PR5)

> Documentation only; no executable verification. **Prepared here, not executed in the spec phase.**

### TASK-6.1 — Gateway contract document

- [ ] Document per-sender `X-Caller-Id` injection, the `401 → ask name (optional) → register_client → retry once` loop, the Telegram `request_contact` step, and the scenario table (A, B + WhatsApp self-chat, first-time client) per `client-registration` requirement "Gateway registration contract is documented"

### TASK-6.2 — Operator/ops documentation (documented, not automated)

- [ ] Document Transfer Ownership of the demo owner phone to the real phone and the `messenger_platform` transition to `whatsapp` as operator steps (non-goals here)
- [ ] Document the new config setting (registration rate limit) and the Telegram owner-bot fields

### TASK-6.3 — PRD status and cross-references

- [ ] Update PRD §7 Fase N status for this item and cross-reference ADR-0018 from the affected docs
- [ ] **Gate:** structural readback (docs gate) — no test execution

---

## Phase 7 — Closing gates (per PR, not per work unit)

- [ ] `go fmt ./...` and `go vet ./...` clean; `golangci-lint run ./...` clean
- [ ] `go build -o /dev/null ./...` passes; `go test -v -race ./...` passes
- [ ] No layering violations: SQL only in `internal/repository`; wire tags only in `internal/mcp`; no role from config; no new dependency
- [ ] Structural readback / native review per the Verification & Review Protocol routing before commit; GGA hook passes; operator approval obtained (never `--no-verify`)

---

## Traceability

| Requirement | Tasks |
|---|---|
| `register_client` is the only anonymous-allowlisted tool; phone from header | 3.1–3.5 |
| `id == phone` creation; byte-identical stored id (`clients` REQ-CL-PORT-001) | 1.1, 1.2, 3.5 |
| Accounts collision rejection; optional name + placeholder; phone format incl. >15-digit rejection | 2.1, 2.2 |
| Per-phone configurable rate limit; audit without full phone | 2.3, 2.4 |
| Only semantic errors reach the client | 2.2, 3.3, 3.4 |
| Anonymous allowlist before resolution (`auth-middleware`) | 3.1, 3.2 |
| Gateway contract documented; operator steps documented only | 6.1, 6.2 |
| Derived `maps_url`; `geo:` alternative input (`business-profile`) | 4.1–4.4 |
| Telegram owner-bot fields + TUI option 7 (`hermes-config-tui`) | 5.1–5.3 |

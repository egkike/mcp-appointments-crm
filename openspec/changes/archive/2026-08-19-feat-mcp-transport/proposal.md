# Proposal: feat-mcp-transport

> **Change**: feat-mcp-transport
> **Phase**: Fase 2 — mcp-server-core (PRD §7)
> **Status**: Proposed — Q1, Q2, Q3 resolved (2026-08-05)
> **Date**: 2026-08-05 (last updated 2026-08-05 with Q1 evidence + Q2/Q3 confirmation)

## Table of Contents

1. [Intent](#1-intent)
2. [Scope](#2-scope)
3. [Approach](#3-approach)
4. [Resolved Decisions](#4-resolved-decisions)
5. [Acceptance Criteria](#5-acceptance-criteria)
6. [Risks](#6-risks)
7. [Out of Scope](#7-out-of-scope)
8. [Estimated Effort](#8-estimated-effort)

---

## 1. Intent

Today `cmd/mcp-server/main.go` is a 146-line composition root: it wires 9 repos + 5 use cases into `_ =` variables and `os.Exit(0)`s. The binary compiles and exits — it does NOT serve the MCP protocol. Hermes has nothing to talk to.

This change makes the binary actually **serve MCP** on `127.0.0.1:3000/mcp` (PRD §3.1, §7 Fase 2 — mcp-server-core). It delivers the transport adapter layer (`internal/mcp/`, mandated by ADR-0013 as the Hexagonal "adapter"), wires the existing `auth.AuthMiddleware` around every request, registers the booking-flow tools against existing use cases, validates loopback binding at startup, and drains gracefully on SIGTERM/SIGINT. After this change, a configured Hermes client can `tools/call` against the server end-to-end.

## 2. Scope

### In Scope
- HTTP server (`net/http`) bound to `127.0.0.1:3000/mcp` (default); `MCP_BIND` + `MCP_PORT` override via env / `~/.config/mcp-appointments-crm/.env` (precedence per ADR-0007).
- MCP protocol implementation — **Streamable HTTP** (spec 2025-11-25), JSON-RPC 2.0 framing, single POST `/mcp` endpoint, `initialize` / `tools/list` / `tools/call`.
- Tool registration for the booking-flow use cases (see Q3 for the exact floor).
- Auth middleware wiring: every `tools/call` carries a verified `Caller` (owner/admin/staff/client) resolved from `X-Caller-Id` and propagated via `context.Context`.
- **Loopback validation at startup**: `MCP_BIND` MUST resolve to `127.0.0.0/8` or `::1`; non-loopback → fail-fast with a clear Spanish error before `ListenAndServe`.
- Graceful shutdown on SIGTERM/SIGINT with bounded in-flight drain (≤10s) via `http.Server.Shutdown`, exceeding the SQLite busy_timeout (5000ms).
- Structured logging (`log/slog`) for request/error lines; auth audit log for owner/admin callers per `auth-middleware` spec.
- Consumer-side interfaces declared in `internal/mcp/` (per `data-access` spec C5: interfaces defined in consumer package).

### Out of Scope
- TUI menú operacional (Fase 2+ follow-up, PRD §3.8.8) — admin subcommand, owner seed.
- HTTPS, TLS, certs (PRD §3.2 — loopback plain HTTP).
- Rate limiting (PRD §3.2 — concurrency handled at SQLite layer).
- Non-loopback listeners (reject at startup, never bind externally).
- Server-to-client notifications (`notifications/tools/list_changed`, etc.).
- Resource subscription, prompt templates, sampling, roots.
- FTS5 search tools, alerts tools, loyalty report (Fase 3).

## 3. Approach

**Recommended: Plan A — `github.com/modelcontextprotocol/go-sdk` (official Go SDK).**

**Evidence** (verified 2026-08-05 via Context7 `/modelcontextprotocol/go-sdk`, versions v0_2_0, v0_4_0, v1.0.0, v1.2.0 — v1.2.0 indicates a stable v1.x API, no longer v0.x alpha):
- SDK exposes `mcp.NewStreamableHTTPHandler(server)` returning a `StreamableHTTPHandler` that implements `http.Handler`. Wiring is `http.Handle("/mcp", handler)` — composes natively with our `auth.AuthMiddleware.Wrap(...)`.
- `StreamableServerTransport` carries `SessionID` + `EventStore` for resumable streams (2025-11-25 spec). The 2025-11-25 revision is the highest the SDK advertises today; the 2026-07-28 "no-sessions/POST-only" revision is NOT yet reflected in the SDK API surface.
- SDK requires Go 1.24+; the project is on Go 1.26.4 ✅.

**Rationale**: stable v1 API, official ownership, native `http.Handler` composition with our existing middleware, JSON-RPC 2.0 + tool registration handled by the SDK, no need to hand-roll a codec. Single external dependency addition to `go.mod`.

**Plan B (fallback, PRD §8 R1)**: hand-rolled JSON-RPC 2.0 over `net/http` with a single `POST /mcp` handler. Triggered only if (a) the SDK cannot be made to reject non-loopback binds cleanly, (b) Hermes requires the 2026-07-28 POST-only revision and the SDK forces session semantics that break Hermes, or (c) the SDK pulls an unacceptable transitive dependency graph. Plan B is ~300 extra LOC (codec + dispatch) but zero new external deps.

**Target spec revision**: 2025-11-25 (Streamable HTTP with sessions). The 2026-07-28 revision (sessions removed, GET returns 405) is a documented downgrade path under risk SUGGESTION below, NOT the target.

## 4. Resolved Decisions

The three open questions identified during the explore phase are now resolved. Evidence and rationale below.

### Q1 — Hermes client transport support — **RESOLVED: Streamable HTTP (default)**

**Finding (2026-08-05)**: The official Hermes Agent documentation ([hermes-agent.nousresearch.com/docs](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp)) and the source code ([NousResearch/hermes-agent `tools/mcp_tool.py`](https://github.com/NousResearch/hermes-agent/blob/main/tools/mcp_tool.py)) confirm that **Hermes v0.20.0 (current) speaks MCP over `stdio` and `HTTP/StreamableHTTP` by default, with `transport: sse` as an opt-in fallback for older servers**.

Key evidence (verbatim from the source):
- *"Hermes supports MCP servers over both `stdio` and HTTP/StreamableHTTP transport"* — MCP integration docs.
- Config: `url: "http://127.0.0.1:3000/mcp"` (no `transport` key) → Hermes uses **Streamable HTTP** automatically.
- `transport: sse` → opt-in to the deprecated SSE transport (GET `/sse` + POST `/messages/`).
- `LATEST_PROTOCOL_VERSION = "2025-03-26"` in the client code; imports `mcp.client.streamable_http.streamablehttp_client` (the canonical Streamable HTTP client).
- PR [#21227](https://github.com/NousResearch/hermes-agent/pull/21227) (merged 2026-05-07) adds SSE support: *"previously forced `StreamableHTTPTransport` for all URL-based MCP servers; servers using SSE transport hung for `connect_timeout`"* — confirming **Streamable HTTP is the default and SSE is the fallback added later**.

**Implication for this change**: Plan A is viable end-to-end. A Hermes user configures `mcp_servers.yaml` with `url: "http://127.0.0.1:3000/mcp"` and the client connects with no extra flags. **No SSE deprecado implementation is needed in this SDD**; if a future change needs to support older Hermes versions that only speak SSE, that is a separate scope.

This finding is persisted in Engram at `sdd/feat-mcp-transport/explore/discovery-hermes-transport-2026-08-05` (obs #664).

### Q2 — PRD terminology drift on "SSE" — **CONFIRMED: in-scope doc fix**

PRD §2.2, §3.1–§3.3, §5.2, §6.1–§6.2, §7 (Fase 2), §8.1–§8.2 y el glosario (todas las menciones de "SSE") + ADR-0007 say "SSE" but the MCP spec deprecates the SSE transport. The "SSE" wording in the project docs is colloquial / outdated. **Decision**: update those PRD sections and ADR-0007 to use "Streamable HTTP (MCP 2025-11-25)" as a small in-scope doc fix in this change. Estimated 10–15 lines of doc edits.

### Q3 — Tool floor (6 vs more) — **CONFIRMED: 6 tools (option a)**

PRD §7 Fase 2 DoD says "6+ tools". Existing use cases in `internal/application/usecase/` back exactly **5 tools**:
1. `check_availability`
2. `create_booking`
3. `get_booking`
4. `cancel_booking`
5. `reschedule_booking`

**Decision**: adopt option (a) — add a trivial `get_business_profile` use case backed by `BusinessProfileRepo.Get`. This:
- Reaches the 6+ floor the PRD asks for.
- Keeps the "handlers consume use cases, not repos" hard rule (ADR-0013 / Fase 2 DoD).
- Is the smallest scope option; `get_or_create_client` (option b) is deferred to Fase 3 with the rest of RF5.

## 5. Acceptance Criteria

- [ ] Server binds ONLY to `127.0.0.1:3000/mcp` (or `MCP_BIND`/`MCP_PORT` override); any non-loopback `MCP_BIND` is rejected at startup with a Spanish error before `ListenAndServe`.
- [ ] `MCP_BIND` = `0.0.0.0` → `Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).` + `os.Exit(1)`; any non-loopback IPv4/IPv6 → `Error: MCP_BIND=<v> no es una dirección loopback. Use 127.0.0.1 (IPv4) o ::1 (IPv6).` (verbatim ADR-0007 §D4, matching `mcp-transport` spec + design).
- [ ] All enumerated tools (per Q3 resolution) exposed via JSON-RPC 2.0 over `POST /mcp` and passing integration tests with `httptest`.
- [ ] `auth.AuthMiddleware` is wrapped around the MCP handler; every `tools/call` carries a verified `Caller` in `context.Context` (verified by a test that asserts `auth.FromContext(ctx)` is populated and that 401/403 paths return Spanish messages per `auth-middleware` spec).
- [ ] Graceful shutdown: SIGTERM/SIGINT → `http.Server.Shutdown(ctx 10s)`; in-flight requests drain or are force-closed at 10s boundary; `database.Close()` runs from the existing `defer`.
- [ ] All business-logic failures return Spanish `*domain.SemanticError` messages; no stack traces or raw SQL leak to the JSON-RPC response (per AGENTS.md coding standards).
- [ ] Tests: unit tests for JSON-RPC framing / tool registration; integration tests for `/mcp` with `httptest`; one e2e test with a mock LLM client doing `initialize` → `tools/list` → `tools/call`; `go test -v -race ./...` clean.
- [ ] `internal/mcp/` declares the consumer interfaces it needs (per `data-access` spec C5); no handler imports `internal/repository/` directly (handlers consume use cases).
- [ ] No new transitive dependency surprises: the ONLY `go.mod` addition is `github.com/modelcontextprotocol/go-sdk` (+ its module-graph); document exact `go.mod` deltas in `tasks.md`. If Plan B is chosen, zero new deps.
- [ ] Pre-flight passes: `gofmt -l .` empty, `go vet ./...` clean, `go build -o /dev/null ./...` passes, `golangci-lint run ./...` clean, `go test -v -race ./...` passes.

## 6. Risks

| # | Severity | Finding | Mitigation |
|---|----------|---------|------------|
| R1 | **CRITICAL** | SDK may not support the 2026-07-28 spec revision (POST-only, no sessions, GET→405). SDK API today exposes `SessionID` + `EventStore` (2025-11-25). | **Mitigation**: target 2025-11-25; Plan B hand-roll fallback if Hermes mandates 2026-07-28. Decision gated at Q1 during apply. |
| R2 | **WARNING** | PRD §3.1 / §9.1 says "SSE" but the SSE transport is deprecated; terminology drift risks reviewer confusion. | **Mitigation**: small PRD doc fix as in-scope deliverable (Q2); spec/design use "Streamable HTTP (MCP 2025-11-25)". |
| R3 | ~~**WARNING**~~ **RESOLVED** | ~~Hermes client may not support Streamable HTTP~~. **Resolved 2026-08-05**: Hermes v0.20.0 client speaks Streamable HTTP by default; verified against official docs + repo (see Q1 in §4 and Engram obs #664). No mitigation needed. | **RESOLVED** — replaced with verification reference to Q1 / obs #664. |
| R4 | **SUGGESTION** | POST-only (2026-07-28) vs POST+GET (2025-11-25) — the SDK today supports sessions (POST+GET). | **Mitigation**: target 2025-11-25 by default; document the downgrade path to 2026-07-28 in `design.md` for when the SDK catches up. |
| R5 | **WARNING** (new) | Plan A adds an external dependency, conflicting with ADR-0005 "no external runtime deps" philosophy. | **Mitigation**: the SDK is compile-time only (Go binary, no runtime deps); document the trade-off in `design.md` and confirm ADR-0005 was scoped to *runtime* deps (which stands). |

## 7. Out of Scope

(Released from §2 — explicit deferrals):
- **TUI menú operacional** (`mcp-appointments-crm admin tui`, owner seed, account CRUD) → Fase 2+ follow-up change.
- **HTTPS / TLS / certs** → PRD §3.2 forbids for loopback.
- **HTTP rate limiting** → PRD §3.2 (SQLite `busy_timeout=5000` + WAL handle contention).
- **Non-loopback listeners** → rejected at startup, never built.
- **Server-to-client MCP notifications** (`notifications/tools/list_changed`, resource updates).
- **Resource subscription, prompt templates, sampling, roots** → not in PRD Fase 2.
- **FTS5 search tools** (`search_clients_advanced`, `search_services_advanced`, RF3) → Fase 3.
- **Alerts tools** (`get_pending_alerts`, `mark_alert_as_sent`, RF7) → Fase 3.
- **Loyalty report** (`get_loyalty_report`, RF8) → Fase 3.
- **`install.sh` + service templates** → Fase 5; this change ships a runnable binary, not a service installer.

## 8. Estimated Effort

Rough breakdown (Plan A path):
- `internal/mcp/` transport + server registration + tool handlers + consumer interfaces: ~450–600 LOC.
- `cmd/mcp-server/main.go` extension (HTTP server, signal handling, shutdown, loopback validation, middleware wiring): ~120 LOC.
- Tests (JSON-RPC framing, `/mcp` integration, e2e mock client, loopback validation, shutdown): ~400–500 LOC test code (not counted in review LOC budget the same way, but stress on PR size).
- PRD "SSE" doc fix (todas las secciones citadas en Q2): ~10 lines.
- **Estimated total production LOC: ~570–720.**

> **LOC reconciliation (superseded by tasks.md)**: this estimate was the pre-tasks range. The authoritative forecast is the `tasks.md` PR Breakdown (848 prod total: PR 1 = 428, PR 2 = 420; 1383 changed lines with tests), which reconciles the per-file detail in `design.md` §2/§9.

**PR strategy** (against the 400-line review budget): the total exceeds 400 LOC. **Recommend**: 2 chained PRs —
1. **PR 1** (~300–350 LOC): `internal/mcp/` transport adapter + server skeleton + loopback validation + JSON-RPC framing + unit tests; binary starts, binds loopback, answers `initialize`/`tools/list` with zero tools, shuts down. No auth wiring yet.
2. **PR 2** (~270–370 LOC): wire `auth.AuthMiddleware`, register the 6 tools against use cases, `/mcp` integration tests + e2e mock-client test, PRD doc fix.

**Decision needed at tasks phase**: stacked-to-main chain vs feature-branch-chain (per `work-unit-commits` / `chained-pr` skills). The orchestrator should surface this choice to the user when planning tasks. If the user prefers a single PR, scope must be trimmed (e.g., defer the e2e mock-client test to Fase 3).

---

## Appendix A — Capabilities (contract for sdd-spec)

> Required by the `sdd-propose` skill. The body sections 1–8 above are authoritative for the orchestrator; this appendix gives `sdd-spec` the capability breakdown so it knows which spec files to create or update.

### New Capabilities
- `mcp-transport`: the Streamable HTTP transport adapter — `internal/mcp/` server, JSON-RPC 2.0 `POST /mcp` endpoint, tool registration, loopback bind validation, graceful shutdown, `log/slog` request/error logging. Becomes `openspec/specs/mcp-transport/spec.md`.

### Modified Capabilities
- `auth-middleware`: wiring contract is fulfilled — `auth.AuthMiddleware.Wrap(...)` is now actually wrapped around the MCP handler at the composition root. The existing in-isolation behavior spec (`openspec/specs/auth-middleware/spec.md`) stays unchanged at the requirement level; the delta spec captures the new "wired" requirement (every `tools/call` carries a verified `Caller`; 401/403 paths surface to JSON-RPC clients in Spanish).
- `architecture`: a new adapter layer (`internal/mcp/`) is added under the Hexagonal model (ADR-0013 C5 — `cmd/` remains the only composition root; new consumer interfaces declared in `internal/mcp/`). The delta spec records the layer addition.
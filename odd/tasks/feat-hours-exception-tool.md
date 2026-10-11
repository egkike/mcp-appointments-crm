# feat-hours-exception-tool — wire BusinessHoursException management for Hermes

**Type**: feature (spec-first) — owner decision 2026-10-11: "Cablear tool MCP ahora"
**Status**: IMPLEMENTED (phase 3) — 3 tools live on the real mux (registered surface at 23); use cases + DTOs (PR1 `2105f9d`), transport tools + ToolRBAC + composition wiring (PR2 `4467f51`), integration tests + docs/spec reconciliation (PR3)
**Branch**: spec `feat/feat-hours-exception-tool-spec` (docs-only, ff-merge to main), then implementation chain

## Scope (from scout evidence 2026-10-11)

The repo already has full persistence for business-hours exceptions (feriados/días especiales): `internal/repository/business_hours_exception.go` — `Create` (:38, RequireRole admin/owner + date/is_closed/time validation + ErrConflict on dup), `Get` (:110, RequireCaller — ALREADY consumed by booking/reschedule/availability), `List` (:145), `Delete` (:200). No use case/tool caller exists for the write/read-list side (application mocks panic). PRD §3.7.3 defines the table + precedence (docs/PRD.md:261-283,554-560); ADR-0006 Decision 2 covers the data model; ADR-0015's maintenance scope does not cover exception editing (neither desired nor deferred — simply uncovered).

## What wiring entails

- Use case(s) (RBAC re-assert + semantic errors, following the 8 existing maintenance use cases' shape), DTOs, transport tools, tool count 20→23 (three separate tools — see Spec phase resolution below), ToolRBAC row + composition root wiring, openspec spec delta (MODIFIED mcp-transport + capability spec for exceptions), integration tests through the real mux.
- Open questions for the spec phase: tool surface granularity (create/delete/list as separate tools vs combined), list semantics (range filter? by-professional? exceptions are business-wide per PRD?), and whether update exists or is delete+create (table has no update per PRD?).

## Spec phase resolution (2026-10-11)

Resolved from repository/PRD evidence while drafting `openspec/changes/feat-hours-exception-tool/`. Full rationale + citations in `proposal.md` Decision Log (D1–D10).

| Question | Resolution | Evidence |
|---|---|---|
| Tool granularity | **Three separate tools**: `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions` (one action per tool) | ADR-0015 delivered one tool per action (`create_service`/`delete_service`/`upsert_schedule`/`delete_schedule` — `cmd/mcp-server/main.go:429-435`); the uncovered repo methods map 1:1 (`internal/domain/repository/business_hours_exception.go:12-24`) |
| Tool count | **20 → 23** (corrected from the earlier 20→21 placeholder) | 20 registered tools in code + README (`internal/mcp/test_helpers_test.go`, `README.md:22`); `20+3=23`. Reaching 21 needs a combined multi-action tool, which contradicts the one-action-per-tool precedent (OQ-1) |
| Exceptions scope | **Business-wide**, never professional-scoped | Table has no `professional_id` (`docs/PRD.md:266-276`); entity has no professional field; read path fetches by date only |
| Update semantics | **No update method/tool**; editing = delete + create | Repo doc comment: "Update is intentionally not provided; the only way to change an exception is Delete + Create (exceptions are immutable by design)" (`internal/repository/business_hours_exception.go:19-22`) |
| List semantics | Inclusive `from`/`to` range, both required, ordered ASC; no invented default window | `List(ctx, from, to time.Time)` already requires both bounds (`internal/repository/business_hours_exception.go:128-134`) |
| RBAC | **Owner-only at both layers** (ToolRBAC `{owner}` + in-usecase `RequireRole(owner)`, double enforcement) | ADR-0015 Decision 2 + the 8 existing maintenance tools (`cmd/mcp-server/main.go:429-435`); repo `Create`/`Delete` keep admin+owner unchanged |
| Rate limiting | Not needed (owner-only, authenticated, low frequency) | Only `register_client` is rate-limited because it is anonymous |

### Open questions — all RESOLVED (owner decision 2026-10-11)

- **OQ-1 — count 20→23 vs the placeholder 20→21. RESOLVED 2026-10-11: accept 23** — one action per tool per ADR-0015; 21 would require a single combined multi-action tool.
- **OQ-2 — capability delta dir. RESOLVED 2026-10-11: singular** — the delta is `specs/business-hours-exception/` (renamed by the orchestrator), matching the base capability `openspec/specs/business-hours-exception/` and the archived-delta convention.
- **OQ-3 — `created_at` on the wire. RESOLVED 2026-10-11: hidden from the wire** — dropped from `hoursExceptionOut`, consistent with `serviceOut` and PRD §513's "sin timestamps"; the SQLite column is unchanged.
- **OQ-4 — tool verb. RESOLVED 2026-10-11: keep `list_hours_exceptions`** (`get_hours_exceptions` rejected).
- **OQ-5 — base spec drift. RESOLVED 2026-10-11: reconcile as a pre-step** — the archived `feat-whatsapp-bot` 19→20 delta (REQ-MT-005 count, REQ-MT-015 `register_client` + `maps_url`/`location_uri`) was backported into `openspec/specs/mcp-transport/spec.md`, so the change's 20→23 delta chains cleanly.

## Process

Spec phase first (proposal/design/spec deltas under `openspec/changes/feat-hours-exception-tool/`), owner review, ff-merge per docs flow; then implementation phases with work units ≤ ~100 lines, chained PRs if over budget. Fallback gate (no native review retry while #3991/#5226 unfixed).

## Spec phase commit (2026-10-11)

- `4835418` — base mcp-transport spec reconciled 19→20 (backport of the feat-whatsapp-bot delta; pre-step for the 20→23 chain).
- `5c9fc39` — change dir committed: proposal (D1–D10 + 5 OQs resolved per owner decisions), design, tasks (4 phases / 9 WUs ≤100 lines), deltas (REQ-MT-005/015 20→23 + REQ-BHE-MCP-001..007, created_at hidden from wire, singular capability dir).
- **Owner review**: 'Dale procede' 2026-10-11 — implementation authorized.
- **NEXT**: phase 1 (TASK-1.1..1.4, PR1) → gate → owner commit approval → PR1 merge → phase 2 (PR2) → phase 3 (PR3). Per tasks.md: OQ-1/OQ-2 resolved (23 tools; singular dir).
- **GGA exception (owner override 2026-10-11)**: the phase-1 commit landed with --no-verify after 3 consecutive STRICT_MODE ambiguous blocks (underlying verdict visibly PASSED in all three). This overrides the tasks.md Phase-4 'never --no-verify' rule BY EXPLICIT OWNER DECISION for this commit only; the review content ran and passed each time. Upstream suggestion recorded: relax the exactly-one-STATUS-line parser (first-STATUS-wins or strip tool-echo lines).
- **PR1**: https://github.com/egkike/mcp-appointments-crm/pull/116 — commit e59ffa8 (6 files, +885: 3 use cases + 23-test suite + DTOs + tracker). Awaiting CI + merge; phase 2 branches from updated main.
- **PR1 MERGED 2026-10-11**: squash `2105f9d` on main, CI green (run 38170537087, 1m07s). Phase 2 branches from this state.
- **PR2**: https://github.com/egkike/mcp-appointments-crm/pull/117 — commit a9903e2 (9 files, +901/−16: 3 tools end-to-end + ToolRBAC + DI + count 20→23 + 16 transport tests). Fixture edits outside WU surfaces (server.go/tools_test.go/server_integration_test.go) reviewed and confirmed by the orchestrator. GGA: --no-verify under owner approval after 2 STRICT_MODE ambiguous blocks (PASSED visible both). Awaiting CI + merge; phase 3 branches from updated main.
- **PR2 MERGED 2026-10-11**: squash `4467f51` on main, CI green (run 38173154617, 1m25s). Phase 3 branches from this state (tool surface at 23 for integration assertions).

## Phase 3 — integration tests + docs/spec reconciliation (2026-10-11)

- `internal/mcp/server_integration_hours_exception_test.go` (new): end-to-end over the real mux (owner creates a CLOSING exception → `check_availability` reports the exact closed semantic `Negocio está cerrado el <fecha> (<motivo>).` → `list_hours_exceptions` returns it inside the inclusive `[date, date]` range → `delete_hours_exception` removes it → availability reverts to the weekly schedule), admin/staff/client rejected with `-32001` on all three tools (pinned from the auth translator, not the tasks.md draft), duplicate date → Spanish conflict message, `tools/list` count 23. Gate `go test -count=1 -race ./internal/mcp/...` green.
- Docs/spec reconciliation: `README.md` (20 → 23 + the three new tools), `docs/PRD.md` §7 Fase N (delivered item, count 23 per OQ-1), `openspec/specs/mcp-transport/spec.md` (REQ-MT-005 20 → 23 + `Previously` chain; REQ-MT-015 three owner-only rows with the delivered wire shapes and `created_at` excluded, plus role/semantics/read-scope notes and the two new scenarios), `openspec/specs/business-hours-exception/spec.md` (REQ-BHE-MCP-001..007 in base framing; REQ-BHE-AUTH-001/002 preserved byte-identical), `internal/mcp/e2e_test.go` stale comment 20 → 23.
- **PR3**: https://github.com/egkike/mcp-appointments-crm/pull/118 — commit ccfdaf0 (integration e2e with revert proof + base-spec reconciliation to 23 + README/PRD). GGA PASSED first attempt. Awaiting CI + merge; after merge the feature closes (change dir archives).
- **PR3 MERGED 2026-10-11**: squash on main, CI green. Change dir archived to `openspec/changes/archive/2026-10-11-feat-hours-exception-tool/`. FEATURE COMPLETE: 3 owner-only tools live (23 total), spec deltas applied to base specs, docs reconciled.

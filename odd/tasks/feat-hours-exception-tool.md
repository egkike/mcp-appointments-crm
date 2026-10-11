# feat-hours-exception-tool — wire BusinessHoursException management for Hermes

**Type**: feature (spec-first) — owner decision 2026-10-11: "Cablear tool MCP ahora"
**Status**: SPEC DRAFTED — `openspec/changes/feat-hours-exception-tool/` (proposal/design/tasks + 2 spec deltas); owner review pending
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

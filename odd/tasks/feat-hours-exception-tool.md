# feat-hours-exception-tool — wire BusinessHoursException management for Hermes

**Type**: feature (spec-first) — owner decision 2026-10-11: "Cablear tool MCP ahora"
**Status**: PLANNED — spec phase not started (batch `followups-backlog-sweep` lands first)
**Branch**: TBD (spec branch `feat/feat-hours-exception-tool-spec`, then implementation chain)

## Scope (from scout evidence 2026-10-11)

The repo already has full persistence for business-hours exceptions (feriados/días especiales): `internal/repository/business_hours_exception.go` — `Create` (:38, RequireRole admin/owner + date/is_closed/time validation + ErrConflict on dup), `Get` (:110, RequireCaller — ALREADY consumed by booking/reschedule/availability), `List` (:145), `Delete` (:200). No use case/tool caller exists for the write/read-list side (application mocks panic). PRD §3.7.3 defines the table + precedence (docs/PRD.md:261-283,554-560); ADR-0006 Decision 2 covers the data model; ADR-0015's maintenance scope does not cover exception editing (neither desired nor deferred — simply uncovered).

## What wiring entails

- Use case(s) (RBAC re-assert + semantic errors, following the 8 existing maintenance use cases' shape), DTOs, transport tool(s), tool count 20→21, ToolRBAC row + composition root wiring, openspec spec delta (MODIFIED mcp-transport + capability spec for exceptions), integration tests through the real mux.
- Open questions for the spec phase: tool surface granularity (create/delete/list as separate tools vs combined), list semantics (range filter? by-professional? exceptions are business-wide per PRD?), and whether update exists or is delete+create (table has no update per PRD?).

## Process

Spec phase first (proposal/design/spec deltas under `openspec/changes/feat-hours-exception-tool/`), owner review, ff-merge per docs flow; then implementation phases with work units ≤ ~100 lines, chained PRs if over budget. Fallback gate (no native review retry while #3991/#5226 unfixed).

# Hermes Maintenance Tools (operational data) — ODD feature

**Status**: IN PROGRESS (started 2026-09-17)
**Scope authority**: ADR-0015 (docs/architecture/0015-hermes-operational-maintenance.md), ADR-0009
**Workflow**: ODD (per owner decision; repo policy is workflow-agnostic per e38d760)
**TDD**: OFF (not configured in this project); every task ships focused tests following existing patterns
**Review budget**: ~400 diff lines per PR → chained PRs expected, explicit `size:exception` if exceeded

## Objective

Give Hermes (the daily operational channel) WRITE tools over business data — profile,
services, professionals, schedules — so that post-install changes stop requiring manual
SQL against `reservas.db`. Repos already have the mutations; this feature is use cases +
MCP wiring + RBAC owner gates (ADR-0015 Context ¶4).

## Scope (ADR-0015 Decision 1 + 2, MVP)

- New WRITE MCP tools: business profile, services, professionals, schedules/agendas.
- RBAC: **owner-only MVP** (ADR-0015 default; partial admin scope NOT granted in this feature).
- Semantic Spanish errors, no internal details (project standard).
- Structured audit log on maintenance mutations (slog, mirroring admin-layer pattern).
- Day-key normalization deferred from admin-tui (owner decision 2026-09-16).

## Non-goals (explicit, do not expand)

- Account management of any kind → TUI only (ADR-0016, defense-in-depth).
- Second install wizard (ADR-0015 rejected alternative).
- Client/booking data edits beyond existing tools.
- Bulk export/import.
- Admin role access to maintenance tools (deferred; owner-only MVP).

## Verified design facts (scout, 2026-09-17)

1. Tool pattern: typed handler closure + local input struct (`json` tags) → `auth.RequireCaller`
   → inline transport validation → `s.cfg.<Port>.Execute(ctx, dto.XxxInput{Caller, ...})`.
   Goto template: `mark_alert_as_sent` (internal/mcp/tools_alerts.go:44-66).
2. RBAC lives in `ToolRBAC` map (cmd/mcp-server/main.go:245) keyed by tool name (path is
   rewritten to tool name by jsonrpcAuthTranslator), plus re-assert via
   `auth.RequireRole(ctx, ...)` inside the use case. Maintenance tools must do BOTH.
3. No use case wraps the maintenance mutations today. Established use-case shape:
   re-inject caller (`auth.WithCaller(ctx, input.Caller)`), `RequireRole`, map sentinels
   (`ErrNotFound` etc.) to `*domain.SemanticError` — else `toMCPError` emits -32603 instead
   of -32002 (internal/mcp/errors.go:40).
4. Existing repo mutations, all guarded `RequireRole(admin, owner)` + entity `Validate()`:
   - `BusinessProfileRepo.Update` (business_profile.go:73) — full entity, RowsAffected 0 ⇒ ErrNotFound
   - `ServicesRepo.Save/Update/Delete` (services.go:31/94/122)
   - `ProfessionalsRepo.Save/Update` (professionals.go:44/158) — Save generates UUID id;
     specialties JSON + `validateSpecialtiesExist`
   - `SchedulesRepo.Upsert/Delete` (schedules.go:98/148) — day_of_week 0..6, HH:MM, start<end
5. Repos take only `*sql.DB` (no logger). Audit today: only `AccountsRepo` (auditAttrs,
   accounts.go:48). Maintenance audit must be added (use-case layer or repo constructor
   change — decided in T2).
6. **Day-key bug found by scout**: `booking_time_validator.go:91` passes `Weekday()`
   (0..6) into `BusinessProfile.GetOpenClose` (keys 1..7, Monday=1). Pre-existing
   mismatch, undetected by current tests. Folded into this feature (deferred day-keys).
   Schedules side (0..6, Sunday=0) is consistent with `time.Weekday`.
7. `internal/validation` is a doc-only stub; no go-playground/validator anywhere.
   Validation stays three-layer hand-written (transport inline + entity.Validate + repo).
8. Tests: MCP unit tests = fn-table mock ports (tools_test.go, `newToolServer`);
   MCP integration = real SQLite harness (server_integration_test.go); repo tests =
   go-sqlmock (`testutil_test.go`). Registry test `TestToolsListElevenTools` will need
   updating as tool count changes.
9. Error convention: `*domain.SemanticError{Code, Message}` with neutral rioplatense
   Spanish, LLM-actionable, no paths/SQL. Transport errors use ErrCodeInvalidInput.

## Design decisions (feature-level, applied during tasks)

- **Tool surface (8 tools)**: `update_business_profile` (partial merge: pointer fields,
  only provided fields updated — LLM-friendly, low blast radius), `create_service`,
  `update_service`, `delete_service`, `create_professional`, `update_professional`,
  `upsert_schedule`, `delete_schedule`. Final names/schemas frozen in T3.
- **Day-key contract for Hermes**: schedules tools expose `day_of_week` 0..6
  (Sunday=0, matches repo + time.Weekday). `business_hours` stays keyed "1".."7"
  (Monday=1) in profile read/write (matches existing `get_business_profile` payload;
  no surface drift). Single translation helper converts between encodings; the
  `booking_time_validator.go:91` consumer is fixed to use it. Tests pin both sides.
- **Validation lives in use case/repo, NOT transport** (ADR-0015 drift mitigation):
  transport only does schema shape checks (types/positivity/lengths) like existing tools.
- **Audit**: structured slog record per maintenance mutation at the use case layer
  (fields: tool, action, entity, entity_id, caller role; caller id hashed like
  hashCallerID pattern). Final placement decided in T2.

## Tasks

- [ ] **T1 — Day-key normalization** (small, independent, unblocks schedule/profile work):
      single translation helper for "1".."7" (Monday=1) ↔ 0..6 (Sunday=0); fix
      `booking_time_validator.go:91` to use it; tests pinning the helper + the validator
      behavior (open/closed by real weekday) + regression coverage for Sunday/Monday.
- [ ] **T2 — Application layer**: DTOs + ports + use cases for the 8 maintenance
      operations; owner-only `RequireRole`; sentinel→SemanticError mapping; structured
      audit slog; use-case unit tests (mock repos).
- [ ] **T3 — MCP wiring**: new registrar (tools_maintenance.go or per-domain files), port
      fields in mcp.Config, ports.go entries, ToolRBAC entries in main.go, repo/use-case
      construction wiring; unit tests with mock ports; update registry-count test.
- [ ] **T4 — Integration tests (real SQLite)**: happy path per tool family; RBAC 403 for
      staff/client (owner 200); semantic error mapping (-32002); day-key contract
      end-to-end (profile hours vs schedule day_of_week vs availability).
- [ ] **T5 — Docs**: README tool list + count, any "11 tools" mentions, PRD changelog row;
      demo-plan smoke wording (the combined VM smoke will use these tools).

## Commits

(recorded per task on feature branch `feat/hermes-maintenance`)

## Notes / deferred

- Partial admin scope for maintenance tools: deferred (ADR-0015 default owner-only).
- `internal/validation` stub implementation: not part of this feature.
- Post-feature: GoReleaser v0.4.0 → combined VM smoke (TUI seed + Hermes maintenance +
  booking flows), per backlog order (obs 910/914).

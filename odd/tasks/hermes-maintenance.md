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

- [x] **T1 — Day-key normalization** ✅ commit `0f64724` (native review review-15171847a628596a
      approved medium/1-lens, 1 non-blocking follow-up; GGA re-run PASSED after 6 hardening fixes):
      single translation helper `entity.ProfileDayKey(time.Weekday)`; fixed the pre-existing
      Sunday-closed bug (raw Weekday 0..6 fed into 1..7-keyed map); GGA pass hardened
      parseBusinessHours (strict strconv keys 1..7) + validateBusinessHoursJSON (HH:MM regex,
      open<close, single unmarshal) + validator fail-closed nil guards / minute-based HH:MM
      comparisons / slot-timezone now; repo fixture "mon"→"1". Tests: helper all 7 weekdays,
      encoding pin, Sunday/Monday/Saturday regressions, invalid-key/malformed-hours tables.
      **Follow-ups (non-blocking):** pluralization "los domingo."→"los domingos" (validator copy);
      R3-001 SUGGESTION business_profile.go:73-76 (informational, reliability lens); Atoi accepts
      "+1"/"01"/"007" as valid keys (minor, read/write share helper, no drift); hhmmToMinutes
      accepts unpadded "9:00" (only write path is regex-guarded seeder); deps.Bookings not
      nil-guarded (programmer-error surface, untested); redundant json.Valid call; map-iteration
      nondeterministic first-error pick.
- [x] **T2 — Application layer** ✅ commit `108a5d9` [size:exception ~2.4k lines] (native review
      review-d5f3c2cd73aa56fc approved high/4-lens, 13 non-blocking findings; GGA passed):
      8 owner-only use cases (update_business_profile partial merge with empty-update rejection;
      create/update/delete service; create/update professional; upsert/delete schedule) + 10 DTOs
      + maintenance_audit.go slog helper + 43 use-case tests. FK classification added at repo layer
      (sqlite_errors.go: 787 + FK-message-guarded 1811; wired at ServicesRepo.Delete /
      SchedulesRepo.Upsert) so delete_service ⇒ "tiene reservas asociadas" (conflict) and
      upsert_schedule ⇒ "el profesional indicado no existe" (not found). Driver facts pinned by
      tests: modernc.org/sqlite RESTRICT emits 1811, dangling FK emits 787.
      **Follow-ups (non-blocking, from 4-lens review):** R2-001 WARNING update_business_profile.go:83-164
      (readability, applyProfileUpdates size); R3-001 WARNING + R4-001 WARNING upsert_schedule.go:64-79
      (read-back semantics / single-source-of-truth); R3-002/R4-002 sqlite_errors.go:94-99 message-guard
      fragility for future triggers; R3-003/R4-003 update_professional.go:84-92 concurrent-delete
      mapping; R4-004 update_business_profile.go:46-60; R2-002..005, R3-004 suggestions. Full list
      burned in review receipt cb99619.
- [x] **T3 — MCP wiring** ✅ commit `3ce2a05` (native review review-b95804a7498af54f approved
      medium/1-lens, 1 non-blocking follow-up R3-001 main.go:357-364; GGA passed): 8 tools
      registered (update_business_profile, create/update/delete_service, create/update_professional,
      upsert/delete_schedule); transport-only shape checks; ports.go/config.go/server.go wiring;
      ToolRBAC rows owner-only in main.go; registry test 11→19; profile write reuses the
      get_business_profile read mapper (identical read/write shape for Hermes).
      **Follow-ups:** e2e_test.go + server_loyalty_integration_test.go still assert 11 tools (fixed in T4);
      main.go "mcp server starting" log counts stale (11 use cases / "usecases", 8); integration
      mux harness wiring owned by T4.
- [ ] **T4 — Integration tests (real SQLite)**: happy path per tool family; RBAC 403 for
      staff/client (owner 200); semantic error mapping (-32002); day-key contract
      end-to-end (profile hours vs schedule day_of_week vs availability).
- [ ] **T5 — Docs**: README tool list + count, any "11 tools" mentions, PRD changelog row;
      demo-plan smoke wording (the combined VM smoke will use these tools).

## Commits

(recorded per task on feature branch)
- T1: `0f64724` on `feat/hermes-maintenance-t1` — fix(domain): normalize day-key encoding and harden booking-time validation
- T2: `108a5d9` — feat(application): maintenance use cases for Hermes tools (T2) [size:exception]

## Notes / deferred

- Partial admin scope for maintenance tools: deferred (ADR-0015 default owner-only).
- `internal/validation` stub implementation: not part of this feature.
- Post-feature: GoReleaser v0.4.0 → combined VM smoke (TUI seed + Hermes maintenance +
  booking flows), per backlog order (obs 910/914).

# Proposal: feat-hours-exception-tool — wire business-hours exception management for Hermes

**Change:** feat-hours-exception-tool
**Status:** proposed (spec phase)
**Inputs:** `odd/tasks/feat-hours-exception-tool.md`; `docs/PRD.md` §3.7.3 / §3.7.13 Paso 3a; ADR-0006 Decision 2; ADR-0015; ADR-0013; ADR-0016
**Other artifacts:** `design.md`, `specs/**`, `tasks.md`
**Evidence base:** every claim below cites `path:line` (full inventory in "Evidence Inventory" below)

---

## Executive Summary

The persistence layer for date-specific business-hours exceptions (feriados, eventos, vacaciones, días con horario reducido) is complete and already consumed on the booking hot path, but it is **write-only-dead**: no MCP tool can create, list or delete an exception, so the owner cannot load a holiday through Hermes and the eight owner-only maintenance tools of ADR-0015 cover profile/services/professionals/schedules but **not** this table (`docs/architecture/0015-hermes-operational-maintenance.md`; `internal/repository/business_hours_exception.go:19-22`).

`BusinessHoursExceptionRepo` exposes four methods:

| Method | Guard | Callers |
|---|---|---|
| `Create` | `RequireRole(admin, owner)` (`internal/repository/business_hours_exception.go:40`) | **none** |
| `Get` | `RequireCaller` (`:102`) | `check_availability` (`internal/domain/service/availability.go:111`) and `create_booking` / `reschedule_booking` via `ResolveSlotContext` (`internal/domain/service/slot_context.go:79` ← `internal/application/usecase/create_booking.go:131`, `internal/application/usecase/reschedule_booking.go:115`) |
| `List` | `RequireCaller` (`:129`) | **none** |
| `Delete` | `RequireRole(admin, owner)` (`:176`) | **none** |

This change closes the gap with the **same wiring-only pattern ADR-0015 used for the other eight maintenance tools** (`docs/architecture/0015-hermes-operational-maintenance.md` Decision 1): three owner-only MCP tools backed by three use cases, the existing repository methods, and the existing prepared-statement SQL. **No schema change, no repository change, no new dependency.** The MCP surface grows from 20 tools to 23.

This is the exact operational hole ADR-0015 set out to close ("corregir un horario, agregar un servicio, desactivar un profesional … es SQL manual" — `docs/architecture/0015-hermes-operational-maintenance.md`) applied to the one maintenance domain ADR-0015 left uncovered. A holiday that arrives by SQL is a holiday the owner cannot set on the day a customer asks.

## Business Problem

- **The owner cannot close a date through the operational channel.** PRD §3.7.3 states the owner loads holidays manually (`docs/PRD.md:285`). Today "manually" means SQL against `reservas.db` — the exact operator friction ADR-0015 was written to remove for every other data family.
- **The precedence rule is live in production but unfed.** `check_availability` consults `business_hours_exception` before the weekly JSON (`docs/PRD.md:554-560`, `573-574`; PRD §3.7.13 Paso 3a). The read path works, but with no write tool the owner has no way to create the row the read path is designed to find.
- **The gap is invisible to the LLM.** Hermes can already `update_business_profile`, `create_service`, `upsert_schedule`, etc. It has no tool that even *mentions* exceptions, so when a customer asks "¿atienden el 25 de diciembre?" the model cannot load the closure; it can only report the weekly schedule and be wrong.
- **Editing has no path either.** The repository deliberately exposes no `Update` (`internal/repository/business_hours_exception.go:19-22`: "exceptions are immutable by design"), so without a delete tool a mistaken exception can only be fixed by SQL.

## Product Outcome

- Hermes can **load a holiday / special day** (`create_hours_exception`), **remove one** (`delete_hours_exception`) and **show the loaded calendar for a date range** (`list_hours_exceptions`), all as owner-only maintenance operations.
- An exception created through the tool immediately governs `check_availability`, `create_booking` and `reschedule_booking`, because those paths already call `BusinessHoursExceptionRepo.Get` (`internal/domain/service/availability.go:111`, `internal/domain/service/slot_context.go:79`). **No read-path change is required for the tool to take effect** — this is the key reason the change is safe and small.
- The MCP surface reaches parity with the persistence layer for this table: every uncovered repo method (`Create`, `List`, `Delete`) gains exactly one tool, and the already-consumed `Get` gains none (its semantics are internal to availability).
- The one-action-per-tool contract of ADR-0015 is preserved (`upsert_schedule` / `delete_schedule` precedent), so the LLM selects a narrow, unambiguous tool per intent.

## Scope

1. **New use cases (owner-only + semantic Spanish errors)** in `internal/application/usecase/`: `create_hours_exception.go`, `delete_hours_exception.go`, `list_hours_exceptions.go`, mirroring the shape of `create_service.go` / `delete_service.go` / `delete_schedule.go` (re-inject caller, `auth.RequireRole(RoleOwner)`, map repository sentinels to `*domain.SemanticError`, one `logMaintenanceMutation` per successful mutation).
2. **New DTOs** in `internal/application/dto/maintenance.go` (`CreateHoursExceptionInput`, `DeleteHoursExceptionInput`, `ListHoursExceptionsInput`, `DeleteHoursExceptionResult`).
3. **New MCP transport tools** in a new `internal/mcp/tools_hours_exception.go`, wired through `internal/mcp/ports.go` + `internal/mcp/config.go`: `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`.
4. **Tool count 20 → 23** (`internal/mcp/test_helpers_test.go` `expectedToolCount`, `README.md:22`, `openspec/specs/mcp-transport/spec.md`).
5. **RBAC double enforcement**: `ToolRBAC` entry `{auth.RoleOwner}` per tool at the composition root (`cmd/mcp-server/main.go:417-436` pattern) **and** `auth.RequireRole(ctx, auth.RoleOwner)` inside each use case (AGENTS.md "double RBAC enforcement"; ADR-0015 Decision 2).
6. **Composition-root DI** in `cmd/mcp-server/main.go` (reuse the existing `bizHoursExRepo`).
7. **Spec deltas**: `specs/mcp-transport/spec.md` (MODIFIED REQ-MT-005 + REQ-MT-015) and the exceptions-capability delta (ADDED requirements for the management surface).
8. **Test coverage on all three layers** (use case, transport/mux, integration through the real mux) — mandatory per AGENTS.md and `openspec/config.yaml` (`apply.tdd: true`).

## Non-goals

- **No schema change.** `business_hours_exception` already exists with the exact DDL of PRD §3.7.3 (`docs/PRD.md:266-276`). No migration.
- **No repository change.** `Create` / `List` / `Delete` are used as-is. In particular, **no `Update` method is added** — the repository documents exceptions as immutable and mandates delete+create (`internal/repository/business_hours_exception.go:19-22`).
- **No professional-scoped exceptions.** The table has no `professional_id` column (`docs/PRD.md:266-274`); exceptions are business-wide by design. This change does not introduce per-professional holidays.
- **No national-holiday library or seed.** PRD §3.7.3 explicitly rejects an automatic holiday library (`docs/PRD.md:285`); the owner loads the dates that matter to their business. No seeded dates ship with this change.
- **No admin-TUI surface.** Editing profile/services/professionals/schedules from the TUI is a rejected alternative in ADR-0016 (`docs/architecture/0016-admin-tui-scope.md:58`, `:110`); exceptions are no exception.
- **No rate limiting.** Owner-only, low-frequency, non-anonymous writes; the rate limiter added for `register_client` is an anonymous-abuse control and has no analog here (see `design.md` §6).
- **No update/edit tool.** Editing is delete+create, exactly as the repository models it.
- **No change to `Get` or to the availability / booking / reschedule read paths.** They already consume `Get` and this change does not touch them.
- **No change to the composition-root `Get`-role semantics** and no new admin grant: ADR-0015's admin partial scope stays deferred (`docs/architecture/0015-hermes-operational-maintenance.md` Decision 2).

## Affected Areas

| Area | Files | Change |
|------|-------|--------|
| Use cases | `internal/application/usecase/create_hours_exception.go` (new), `delete_hours_exception.go` (new), `list_hours_exceptions.go` (new) | Owner-only gate, sentinel→semantic mapping, audit event per mutation |
| DTOs | `internal/application/dto/maintenance.go` | Input/result carriers with cross-layer doc comments |
| Transport | `internal/mcp/tools_hours_exception.go` (new), `internal/mcp/ports.go`, `internal/mcp/config.go` | Three typed tools, three consumer ports, three `Config` fields |
| Composition root | `cmd/mcp-server/main.go` | Three `ToolRBAC` rows (`{owner}`), three DI constructors, three `Config` fields |
| Test harness | `internal/mcp/test_helpers_test.go` | `expectedToolCount` 20 → 23 |
| Docs | `README.md`, `docs/PRD.md` §7 Fase N | Tool count + backlog status |
| Spec deltas | `specs/mcp-transport/spec.md` (MODIFIED, 20 → 23), capability delta (ADDED) | Requirements + scenarios |
| Tests | alongside each file | Use case unit tests, transport tests, integration through the real mux |

## Decision Log

Each decision is resolved from repository/PRD evidence; ambiguous or conflicting items are recorded as **Open Questions** in `design.md` §8 and in the report, never invented.

| # | Decision | Evidence and rationale |
|---|----------|------------------------|
| D1 | **Granularity: three separate tools** — `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`. One action per tool. | ADR-0015 delivered the maintenance surface as one tool per action (`create_service`/`update_service`/`delete_service`, `upsert_schedule`/`delete_schedule` — `cmd/mcp-server/main.go:429-435`); no existing tool multiplexes actions behind an `action` field. The uncovered repo methods map 1:1 onto the three tools (`Create`→create, `List`→list, `Delete`→delete — `internal/domain/repository/business_hours_exception.go:12-24`). Narrow tools also give the model an unambiguous choice per intent. **Consequence:** tool count is **20 → 23**. This contradicts the task-file placeholder "20 → 21" (see Open Question OQ-1); reaching 21 would require a single combined multi-action tool, which contradicts the ADR-0015 precedent. **Resolved (owner decision 2026-10-11): accept 23**, one action per tool per ADR-0015. |
| D2 | **Exceptions are business-wide, never professional-scoped.** | The DDL declares `exception_date` UNIQUE and no `professional_id` (`docs/PRD.md:266-276`); `entity.BusinessHoursException` has no professional field (`internal/domain/entity/business_hours_exception.go:6-14`); the read path fetches by date only (`internal/domain/service/availability.go:111`). No scoping parameter is added. |
| D3 | **No update tool; editing = delete + create.** | The repository states it explicitly: "Update is intentionally not provided; the only way to change an exception is Delete + Create (exceptions are immutable by design)" (`internal/repository/business_hours_exception.go:19-22`). Adding an update tool would contradict the documented immutability model and require a new repo method (out of scope: no repository change). |
| D4 | **List takes an inclusive `from`/`to` range; no "upcoming N days" default.** | `List(ctx, from, to time.Time)` already requires both bounds and returns `exception_date` ascending (`internal/repository/business_hours_exception.go:128-139`). Mirroring the repo contract adds zero new semantics; inventing a default window (e.g. "next 90 days") would be new product behavior the PRD does not authorize. The PRD's precedence model needs only point lookups (`Get`), so `list` exists for owner visibility and id discovery, not for the hot path. |
| D5 | **RBAC: owner-only at both layers** (composition-root `ToolRBAC {auth.RoleOwner}` + in-usecase `auth.RequireRole(ctx, auth.RoleOwner)`), for all three tools. | ADR-0015 Decision 2 fixes the MVP maintenance RBAC as owner-only and defers the partial admin scope (`docs/architecture/0015-hermes-operational-maintenance.md`); the eight existing maintenance tools are `{auth.RoleOwner}` (`cmd/mcp-server/main.go:429-435`). AGENTS.md requires **double enforcement** (permission map + in-usecase check). **Layering note:** the repository's own `Create`/`Delete` guard is `RequireRole(admin, owner)` (`internal/repository/business_hours_exception.go:40`, `:176`) and is **left unchanged**; the owner-only use-case gate means an admin never reaches the repository through MCP — the same asymmetry that already exists for the eight ADR-0015 tools. |
| D6 | **No rate limiting.** | The only rate-limited surface is `register_client`, because it is reachable anonymously (`internal/application/usecase/registration_rate_limiter.go`). ADR-0015 maintenance tools carry no limiter and neither will these: the caller is an authenticated owner and the writes are low-frequency. |
| D7 | **Audit: one `logMaintenanceMutation` per successful create/delete; none for list.** | `internal/application/usecase/maintenance_audit.go` defines the contract: exactly one structured record per successful **mutation**, emitted at the use-case layer. `list_hours_exceptions` is a read and emits nothing, matching `get_business_profile`. Entity name: `business_hours_exception`; actions `create` / `delete`. |
| D8 | **Validation stays in the repository; the use case maps sentinels to semantic Spanish errors (no duplicated rule set at the transport).** | ADR-0015's drift mitigation puts business validation in the use case/repository, not the transport (`docs/architecture/0015-hermes-operational-maintenance.md`; `internal/mcp/tools_maintenance.go:23-33`). The repository already validates date format, `is_closed`/time consistency, `HH:MM` and `open < close` (`internal/repository/business_hours_exception.go:44-75`). The use case passes through the repository's `*domain.SemanticError` date messages and maps the remaining sentinels (`ErrInvalidInput`, `ErrConflict`, `ErrNotFound`) to LLM-actionable Spanish. |
| D9 | **Tool names follow the CRUD vocabulary: `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`.** | Verb-per-action mirrors the existing tool names; "list" mirrors the repo method `List` and reads naturally with the plural collection. Existing reads use `get_*`/`search_*`, so the `list_*` verb has no precedent — recorded as Open Question OQ-4 with `get_hours_exceptions` as the alternative. **Resolved (owner decision 2026-10-11): keep `list_hours_exceptions`.** |
| D10 | **`create_hours_exception` returns the stored row (read-back) including its `id`.** | `repo.Create` returns only `error` and does not populate `ex.ID` (AUTOINCREMENT — `internal/repository/business_hours_exception.go:77-84`). The maintenance convention is "read back exactly what you wrote" (`update_business_profile` / `create_service` return the stored entity; `internal/mcp/tools_maintenance.go:14-32`). A read-back via the existing `Get(date)` gives the LLM the id it needs to delete, with **no repository change**. |

## Risks & Rollback

| # | Risk | Mitigation |
|---|------|------------|
| R1 | **Booking suppression (DoS) by planting a far-future closing exception.** A row with `is_closed = 1` makes `check_availability` reject every slot that day (`docs/PRD.md:573-574`). | This is precisely why the repository already restricts `Create`/`Delete` to admin/owner (`internal/repository/business_hours_exception.go:40`, `:176`, documented as "DoS prevention" in `openspec/specs/business-hours-exception/spec.md` REQ-BHE-AUTH-001). The new MCP surface narrows it further to **owner-only** at both layers (D5), so no role below owner can reach it. Audited (D7). |
| R2 | **Validation drift between the new tool and the repository rules.** | No rule is re-implemented: the repository remains the single validator (D8), and the use case only *maps* its errors. `update_business_profile`/`upsert_schedule` set the precedent of using `entity.ValidHHMM` at the edge **in addition to** the repo; this change deliberately does not duplicate the date/`is_closed` rules at the transport. |
| R3 | **Tool-count bookkeeping drifts again** (README, `expectedToolCount`, the base spec). | `expectedToolCount` is the single source of truth in the test suite (`internal/mcp/test_helpers_test.go`), and `TestServerToolCountTracksRegisteredTools` pins it to `Server.ToolCount()`. This change updates `expectedToolCount` 20 → 23 in the same PR as the tools, and reconciles `README.md:22` and the base spec — see R5. |
| R4 | **`create` read-back returns a stale/failed row.** | The read-back uses `Get(date)` immediately after a successful `Create`; a read failure is returned as an infrastructure error (`-32603`), never as a half-populated payload — the transport's `errNilMaintenanceEntity` fail-closed guard covers the nil case (`internal/mcp/tools_maintenance.go:39`). |
| R5 | **The checked-in base transport spec is already stale (reads 19 tools, lacks `register_client`).** | Verified: `openspec/specs/mcp-transport/spec.md:60` "19 tools"; the 19→20 delta lives only in `openspec/changes/archive/2026-10-08-feat-whatsapp-bot/specs/mcp-transport/spec.md`, while `README.md:22` and `internal/mcp/test_helpers_test.go` both say 20. The base spec must be reconciled (apply the archived whatsapp delta) before/at archive of this change; the delta here is authored against the **code truth (20)** and states 20 → 23. Flagged as Open Question OQ-5. **Resolved (owner decision 2026-10-11): backport applied to the base spec as a pre-step of this change** (19 → 20), so the 20 → 23 delta now chains cleanly. |
| R6 | **Review budget.** ~600–800 changed lines across 3 layers. | Work units capped at ~100 lines in `tasks.md`; a chained-PR split is pre-planned (use cases → transport+wiring → integration+docs). `openspec/config.yaml` `review_budget_lines: 400` ⇒ chained PRs are the default strategy. |

**Rollback:** every piece is additive and independently revertible. Removing the three tool registrations + `ToolRBAC` rows + DI restores the current 23→20 surface with no data effect; the repository, schema and read paths are untouched, and any exception rows created meanwhile remain valid and are still honored by `check_availability`. **No migration, no destructive step, nothing to reverse in the database.**

## Success Criteria

- [ ] `tools/list` returns 23 tools, including `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`; `expectedToolCount` is 23 and `README.md` matches.
- [ ] `create_hours_exception` with `{exception_date: "2026-12-25", is_closed: true, reason: "Navidad"}` persists the row and returns it with its generated `id`.
- [ ] `create_hours_exception` with `is_closed: false` and valid `open_time`/`close_time` persists the custom window; with missing/invalid times it fails with a semantic Spanish error and no row is written.
- [ ] A duplicate `exception_date` fails with a semantic Spanish `ErrCodeConflict` message (`-32002`) and does not modify the existing row.
- [ ] `delete_hours_exception {exception_id}` removes the row; an unknown id fails with a semantic Spanish not-found message (`-32002`).
- [ ] `list_hours_exceptions {from, to}` returns the rows in the inclusive range ordered by `exception_date` ASC; an empty range returns an empty array (not an error).
- [ ] Each successful create/delete emits exactly one `logMaintenanceMutation` audit record (`entity = business_hours_exception`); `list` emits none.
- [ ] All three tools reject staff/client callers with `-32001` at the transport **and** would reject them in the use case (double enforcement asserted in tests).
- [ ] An exception created through the tool changes `check_availability` for that date (integration test through the real mux), proving the read path honors the wire-created row.
- [ ] Only `*domain.SemanticError` messages reach the client; no repository/SQL detail leaks (`internal/mcp/errors.go:41-48`).
- [ ] No repository file and no schema file changes; no new dependency.
- [ ] `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...` clean; `go build -o /dev/null ./...` passes; `go test -v -race ./...` passes.

## Open Questions (owner) — all RESOLVED (owner decision 2026-10-11)

- **OQ-1 — Tool count 20 → 21 vs 20 → 23. — RESOLVED 2026-10-11: accept 23.** Three separate tools, one action per tool per ADR-0015; the task-file "20 → 21" placeholder is superseded and survives only as a historical note.
- **OQ-2 — Delta directory name. — RESOLVED 2026-10-11: singular.** The capability delta is `specs/business-hours-exception/`, matching the base capability `openspec/specs/business-hours-exception/` and the archived-delta convention. The plural supplied by the edit surface is superseded.
- **OQ-3 — `created_at` on the wire. — RESOLVED 2026-10-11: hidden from the wire**, like the `serviceOut` timestamps and per the PRD §513 "sin timestamps" convention; the column itself is unchanged in SQLite.
- **OQ-4 — Tool name verb. — RESOLVED 2026-10-11: keep `list_hours_exceptions`** (mirrors `List`, unambiguous as a collection read; `get_hours_exceptions` rejected).
- **OQ-5 — Base transport spec drift. — RESOLVED 2026-10-11: reconcile as a pre-step.** The archived `feat-whatsapp-bot` 19 → 20 delta was backported into `openspec/specs/mcp-transport/spec.md`, so the base spec now describes the shipped 20 tools and this change's 20 → 23 delta chains cleanly.

## Next

- `design.md` — register/delete/list flows, validation reuse, semantic error mapping, audit, security review, open questions.
- `specs/mcp-transport/spec.md` — MODIFIED delta: `tools/list` 20 → 23 + the three registry rows.
- capability delta — ADDED requirements for the owner-only management surface.
- `tasks.md` — phased work units ≤ ~100 lines with a Review Workload Forecast.

---

## Evidence Inventory

| Evidence | Location |
|---|---|
| Exception table + `is_closed` semantics + precedence (business-wide, date-keyed) | `docs/PRD.md:261-285` |
| Availability precedence step 3a (exception first, JSON fallback) | `docs/PRD.md:554-560`, `:573-574`, `:706` |
| Exception lookup index `(exception_date) UNIQUE` | `docs/PRD.md:489` |
| "no timestamps" note (contradicts the DDL) | `docs/PRD.md:513` |
| ADR-0006 Decision 2: separate table, JSON fallback, rejected alternatives | `docs/architecture/0006-data-model-and-reservations.md:49-76` |
| ADR-0015: maintenance tools as wiring, owner-only MVP, semantic Spanish errors, audit | `docs/architecture/0015-hermes-operational-maintenance.md` (Decisions 1–3) |
| ADR-0016: no profile/services/schedules editing in the TUI | `docs/architecture/0016-admin-tui-scope.md:58`, `:110` |
| Repo "no Update, immutable by design" | `internal/repository/business_hours_exception.go:19-22` |
| Repo `Create` guard + validation rules (date, `is_closed`, `HH:MM`, `open<close`, dup→conflict) | `internal/repository/business_hours_exception.go:39-84` |
| Repo `Get` guard + consumption by availability | `internal/repository/business_hours_exception.go:101`; `internal/domain/service/availability.go:111` |
| Repo `Get` consumption by create/reschedule | `internal/domain/service/slot_context.go:79` ← `internal/application/usecase/create_booking.go:131`, `internal/application/usecase/reschedule_booking.go:115` |
| Repo `List` guard + inclusive range + ordering | `internal/repository/business_hours_exception.go:128-139` |
| Repo `Delete` guard + `ErrNotFound` | `internal/repository/business_hours_exception.go:175-198` |
| Port contract | `internal/domain/repository/business_hours_exception.go:12-24` |
| Entity (no `Validate`, no professional field) | `internal/domain/entity/business_hours_exception.go:6-27` |
| Date validation returns `*domain.SemanticError`; `HH:MM` regex | `internal/repository/validation.go:30`, `:62-80` |
| Maintenance use-case shape (owner gate, sentinel mapping, audit, read-back) | `internal/application/usecase/create_service.go`, `delete_service.go`, `delete_schedule.go` |
| Audit contract (`logMaintenanceMutation`) | `internal/application/usecase/maintenance_audit.go:44-72` |
| Maintenance DTO carriers | `internal/application/dto/maintenance.go` |
| Transport tool/port/config pattern | `internal/mcp/tools_maintenance.go`, `internal/mcp/ports.go`, `internal/mcp/config.go:32-58` |
| Semantic-error passthrough + `-32002` | `internal/mcp/errors.go:33-90` |
| Composition-root RBAC map + DI | `cmd/mcp-server/main.go:390-436`, `:459-487` |
| Tool-count single source of truth (20) | `internal/mcp/test_helpers_test.go` |
| README tool count (20) | `README.md:22-26` |
| Base transport spec drift (19) | `openspec/specs/mcp-transport/spec.md:60`, `:125-142` |
| Archived 19→20 delta | `openspec/changes/archive/2026-10-08-feat-whatsapp-bot/specs/mcp-transport/spec.md` |
| Existing capability (repo-layer auth requirements) | `openspec/specs/business-hours-exception/spec.md` (REQ-BHE-AUTH-001/002) |
| Change-dir ↔ base-capability naming convention | all `openspec/changes/archive/*/specs/*/` directories |

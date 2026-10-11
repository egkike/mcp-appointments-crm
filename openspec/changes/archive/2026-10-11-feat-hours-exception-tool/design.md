# Design: feat-hours-exception-tool

**Change:** feat-hours-exception-tool
**Inputs:** `proposal.md` (D1–D10), `docs/PRD.md` §3.7.3 / §3.7.13 Paso 3a, ADR-0006 Decision 2, ADR-0015, ADR-0013, ADR-0016
**Constraint:** no new third-party dependency; no schema change; no repository change; no read-path change. Everything below reuses existing stdlib + existing code seams.

---

## 0. Layering contract (non-negotiable, ADR-0013)

| Layer | Location | This change adds |
|-------|----------|------------------|
| Domain | `internal/domain/repository`, `internal/domain/entity` | **Nothing.** The port already declares `Get`/`Create`/`List`/`Delete` and the entity has no `Validate`. |
| Application | `internal/application/usecase/create_hours_exception.go`, `delete_hours_exception.go`, `list_hours_exceptions.go` (new) + `internal/application/dto/maintenance.go` | Orchestration: owner gate, sentinel→semantic mapping, audit (mutations only), read-back for create |
| Transport | `internal/mcp/tools_hours_exception.go` (new), `internal/mcp/ports.go`, `internal/mcp/config.go` | Typed input/output DTOs (JSON tags), three tool registrations |
| Persistence | `internal/repository/business_hours_exception.go` | **Nothing.** Reused as-is. |
| Composition root | `cmd/mcp-server/main.go` | Three `ToolRBAC` rows (`{owner}`) + three DI constructors + three `Config` fields |
| Test harness | `internal/mcp/test_helpers_test.go` | `expectedToolCount` 20 → 23 |

Wire JSON tags exist only in `internal/mcp`; SQL only in `internal/repository`; RBAC is enforced twice (composition-root map + in-usecase `RequireRole`). No outer→inner dependency is added.

---

## 1. `create_hours_exception`

### 1.1 Flow

```
tools/call create_hours_exception
  │
  ├─ ToolRBAC["create_hours_exception"] = {owner}                 (composition root, cmd/mcp-server/main.go)
  │     └─ not an owner ─► -32001 "no tienes permiso para realizar esta acción" (auth_translator)
  │
  ├─ handler: decode createHoursExceptionIn (typed struct, JSON tags)
  │     └─ auth.RequireCaller(ctx) ─► caller                        (same guard every maintenance tool uses)
  │
  ├─ use case CreateHoursExceptionUseCase.Execute(ctx, dto.CreateHoursExceptionInput)
  │     ├─ ctx = auth.WithCaller(ctx, input.Caller)
  │     ├─ auth.RequireRole(ctx, auth.RoleOwner)                    (in-usecase re-assert, AGENTS.md)
  │     ├─ build *entity.BusinessHoursException{ExceptionDate, IsClosed, OpenTime, CloseTime, Reason}
  │     ├─ repo.Create(ctx, ex)
  │     │     ├─ RequireRole(admin, owner)                           (repo layer, unchanged)
  │     │     ├─ validateExceptionDate → *domain.SemanticError       (clean Spanish message)
  │     │     ├─ is_closed/time consistency → wrapped ErrInvalidInput
  │     │     └─ duplicate date → wrapped domain.ErrConflict
  │     ├─ error mapping (see §4)
  │     ├─ read-back: repo.Get(ctx, date) ─► stored row (gives the AUTOINCREMENT id)
  │     └─ logMaintenanceMutation(logger, caller, "create_hours_exception", "business_hours_exception", strconv.Itoa(id), "create")
  │
  └─ output: hoursExceptionOut (the stored row)
```

**Why read-back:** `repo.Create` returns only `error` and never assigns `ex.ID`; the SQLite `AUTOINCREMENT` id is invisible to the caller otherwise (`internal/repository/business_hours_exception.go:77-84`). The maintenance convention is "return the row as stored" (`create_service` returns the saved entity; `update_business_profile` re-reads). A `Get(date)` immediately after a successful `Create` is a single indexed lookup (the `exception_date` UNIQUE index — `docs/PRD.md:489`) and needs **no repository change** (D10). The `id` is what the LLM needs to call `delete_hours_exception`.

### 1.2 Wire shape (transport)

| Wire field | Type | Required | Notes |
|---|---|---|---|
| `exception_date` | `string` | yes | `YYYY-MM-DD`, no time component (PRD §3.7.3) |
| `is_closed` | `bool` | yes | `true` = business closed that day; `false` = open with custom window |
| `open_time` | `*string` | conditional | required when `is_closed = false`, forbidden when `true`, `HH:MM` 24h |
| `close_time` | `*string` | conditional | same rule as `open_time`; `open_time < close_time` |
| `reason` | `*string` | no | free text ("Navidad", "Vacaciones del dueño") |

Output (`hoursExceptionOut`, shared with `list_hours_exceptions`): `{id, exception_date, is_closed, open_time?, close_time?, reason?}`. Timestamps are **hidden from the wire** per the PRD §513 convention ("sin timestamps"), mirroring `serviceOut`; the `created_at` column stays in SQLite and in the repository row.

### 1.3 Validation reuse (single validator, no drift)

The repository is the **only** validator, per ADR-0015's drift mitigation ("validación en el use case/repo, no en el transporte") and the transport's own stated split (`internal/mcp/tools_maintenance.go:23-33`). Rules already enforced inside `repo.Create` (`internal/repository/business_hours_exception.go:44-75`):

| Rule | Enforcement | Failure |
|---|---|---|
| `exception_date` matches `^\d{4}-\d{2}-\d{2}$` and is a real calendar date | `validateExceptionDate` (`internal/repository/validation.go:62-80`) | `*domain.SemanticError{ErrCodeInvalidInput}` with clean Spanish message |
| `is_closed = true` ⇒ `open_time`/`close_time` MUST be nil | repo `Create` | wrapped `domain.ErrInvalidInput` |
| `is_closed = false` ⇒ both times present | repo `Create` | wrapped `domain.ErrInvalidInput` |
| both times match `^([01]\d|2[0-3]):([0-5]\d)$` | `timeHHMMRe` (`internal/repository/validation.go:30`) | wrapped `domain.ErrInvalidInput` |
| `open_time < close_time` (lexicographic, valid for zero-padded `HH:MM`) | repo `Create` | wrapped `domain.ErrInvalidInput` |
| `exception_date` UNIQUE | SQLite index + `isUniqueViolation` (`internal/repository/business_hours_exception.go:83-84`) | wrapped `domain.ErrConflict` |

The transport handler does **not** duplicate these rules. It only relies on the SDK's struct-tag validation for required fields (`exception_date`, `is_closed`), which fails as the SDK protocol error (`-32602`) exactly like every other tool. This is deliberate: `upsert_schedule` re-checks `HH:MM` at the edge for a better message, but the exception validation is multi-rule and stateful (`is_closed` ↔ times), so duplicating it would be the drift ADR-0015 warns about.

> **Trade-off:** the LLM loses per-rule message precision (a single `ErrInvalidInput` maps to one message enumerating the rules — §4) in exchange for a single source of truth. If message precision is later required, the repository's per-rule branches can be promoted to `*domain.SemanticError` one by one without touching the tool.

### 1.4 Use case order of operations

1. `ctx = auth.WithCaller(ctx, input.Caller)`; `auth.RequireRole(ctx, auth.RoleOwner)` → `-32001` semantic error on failure (shape copied from `create_service.go:38-43`).
2. Build the entity from the DTO. `OpenTime`/`CloseTime`/`Reason` travel as `*string` (the entity stores pointers — `internal/domain/entity/business_hours_exception.go:10-12`), so an omitted value stays `NULL` in SQLite as required by `is_closed = 1`.
3. `repo.Create(ctx, ex)`.
4. Map the error per §4.
5. `repo.Get(ctx, date)` for the read-back; `nil` result is an infrastructure failure (fail closed — never emit a half-empty payload; same class as `errNilMaintenanceEntity`, `internal/mcp/tools_maintenance.go:39`).
6. `logMaintenanceMutation(...)`, `action = "create"`, `entity_id = <id>`.
7. Return the row.

No `Update` step exists (D3): changing an exception is `delete_hours_exception` + `create_hours_exception`.

---

## 2. `delete_hours_exception`

```
tools/call delete_hours_exception {exception_id}
  ├─ ToolRBAC = {owner}
  ├─ handler: auth.RequireCaller(ctx); decode deleteHoursExceptionIn{exception_id int}
  ├─ use case: WithCaller → RequireRole(RoleOwner)
  ├─ repo.Delete(ctx, id)                                  (RequireRole(admin,owner) repo-layer, unchanged)
  │     ├─ sql.ErrNoRows/RowsAffected==0 ─► domain.ErrNotFound
  │     └─ infra error ─► *domain.SemanticError{ErrCodeInternal}
  ├─ map: ErrNotFound ─► SemanticError{ErrCodeNotFound, "no existe una excepción con ese id"}
  └─ logMaintenanceMutation(..., "delete_hours_exception", "business_hours_exception", strconv.Itoa(id), "delete")
  └─ output: {exception_id, status: "deleted"}
```

- **Identity:** `exception_id` (int), mirroring `repo.Delete(ctx, id int)` (`internal/repository/business_hours_exception.go:175`). Delete-by-date is rejected as an alternative: it would require a `Get` + `Delete` composition in the use case and would make the row identity implicit; exposing the id is already the pattern for `scheduleOut` and `delete_service`'s `service_id`.
- **Result shape:** `{exception_id, status: "deleted"}` mirrors `dto.DeleteServiceResult` / `dto.DeleteScheduleResult` (`internal/application/dto/maintenance.go`).

---

## 3. `list_hours_exceptions`

```
tools/call list_hours_exceptions {from, to}
  ├─ ToolRBAC = {owner}
  ├─ handler: auth.RequireCaller(ctx); decode listHoursExceptionsIn{from, to string}
  ├─ use case: WithCaller → RequireRole(RoleOwner)
  │     ├─ parse from/to as YYYY-MM-DD (transport-level format gate, see below)
  │     └─ repo.List(ctx, from, to)   ── RequireCaller repo-layer (any authenticated), unchanged
  └─ output: []hoursExceptionOut ordered by exception_date ASC
```

- **Range semantics:** `from` and `to` are **both required** and **inclusive**, exactly mirroring `List(ctx, from, to time.Time)` (`internal/repository/business_hours_exception.go:128-141`, "within the [from, to] date range (inclusive), ordered by exception_date ascending"). No default window is invented (D4).
- **Empty range is not an error:** an empty result is `[]`, never `ErrNotFound` — `List` returns an empty slice, unlike `Get` which returns `ErrNotFound` for a missing date.
- **Date parse gate:** `from`/`to` must be parsed into `time.Time` before the repo call. A malformed bound is rejected with a semantic Spanish error (`ErrCodeInvalidInput`) at the use case (or transport), reusing the same `YYYY-MM-DD` rule the repository applies to `exception_date`. This is a format-only gate; it does not duplicate business validation.
- **RBAC note:** the repository allows any authenticated caller to `List` (`REQ-BHE-AUTH-002`), because `Get`/`List` sit on the availability hot path. The **MCP tool is still owner-only** (D5): exception listing is an owner maintenance view, and `check_availability` already accounts for exceptions internally for every role. The repo-layer requirement is unchanged; only the MCP surface is narrowed.
- **No audit:** `list` is a read (`internal/application/usecase/maintenance_audit.go` audits mutations only).

---

## 4. Semantic error mapping (Spanish, LLM-actionable)

The transport's error contract keeps **only** `*domain.SemanticError` messages (`internal/mcp/errors.go:41-48`); everything else collapses to `-32603` "error interno del servidor". The use cases therefore map repository sentinels to semantic errors:

| Repository outcome | Use-case mapping | Client message (example) |
|---|---|---|
| `*domain.SemanticError` (date format/calendar) | **pass through unchanged** | `Error: la fecha debe tener formato YYYY-MM-DD.` / `Error: la fecha no es una fecha válida (formato YYYY-MM-DD).` |
| `ErrInvalidInput` (is_closed↔times, `HH:MM`, `open<close`) | `ErrCodeInvalidInput` | `Error: la excepción no es válida. Si el negocio está cerrado, no envíes open_time/close_time; si está abierto, ambos son obligatorios en formato HH:MM y open_time debe ser anterior a close_time.` |
| `ErrConflict` (duplicate date) | `ErrCodeConflict` | `Error: ya existe una excepción para esa fecha. Borrala y creala de nuevo si querés cambiar el horario de ese día.` |
| `ErrNotFound` (delete unknown id) | `ErrCodeNotFound` | `Error: no existe una excepción con ese id.` |
| any other error | returned wrapped (`fmt.Errorf`) → `-32603` | generic server error; detail only in server logs |

The pass-through branch is checked first (`errors.As(err, &sem)`), because `validateExceptionDate` already returns a `*domain.SemanticError` with a clean Spanish `Message` and a `Cause` that keeps `domain.ErrInvalidInput` reachable through `errors.Is` (`internal/repository/validation.go:62-80`). Every other branch **replaces** the repository's internal prefix with a client-facing message, so no `"crear excepción: ..."` prefix or driver text reaches the LLM.

This mapping is copied from the existing maintenance precedent (`create_service.go:69-81`, `delete_schedule.go:42-52`, `delete_service.go:44-55`).

---

## 5. Audit events

Exactly one record per successful **mutation**, emitted at the use-case layer via `logMaintenanceMutation` (`internal/application/usecase/maintenance_audit.go:44-72`):

| Tool | `entity` | `entity_id` | `action` |
|---|---|---|---|
| `create_hours_exception` | `business_hours_exception` | the new row id | `create` |
| `delete_hours_exception` | `business_hours_exception` | the deleted id | `delete` |
| `list_hours_exceptions` | — | — | *(none — read)* |

No caller ID is logged (the helper's contract: raw caller IDs are PII and the auth middleware already emits a hashed privileged-access record). No exception content (reason text) is logged beyond the id.

---

## 6. Security review

| Surface | Control | Evidence |
|---|---|---|
| **Booking suppression (DoS)** by planting a closing exception | Owner-only at the ToolRBAC layer **and** in the use case; the repository's admin+owner guard remains as the inner barrier. The risk is already documented as the reason `Create`/`Delete` are gated (`openspec/specs/business-hours-exception/spec.md` REQ-BHE-AUTH-001: "any caller able to write exceptions can suppress bookings"). This change does not widen the set of callers — it narrows it to owner. | `internal/repository/business_hours_exception.go:39`, `:175`; ADR-0015 Decision 2; AGENTS.md double enforcement |
| **Rate limiting** | Not required: authenticated owner, low frequency, no anonymous reachability. The `register_client` limiter exists because that path is anonymous (`internal/application/usecase/registration_rate_limiter.go`); there is no equivalent threat model here. | D6 |
| **SQL injection** | No new SQL. The repository binds every value with `?` (`internal/repository/business_hours_exception.go:77-79`, `:137-139`, `:179-180`). | prepared-statements rule |
| **Input validation** | `exception_date` format/calendar, `HH:MM`, `open<close`, `is_closed`↔times consistency, uniqueness — all enforced by the repository before any write. The SDK validates required fields at the edge. | `internal/repository/validation.go:30`, `:62-80`; `internal/repository/business_hours_exception.go:44-75` |
| **Error leakage** | Only `*domain.SemanticError` messages reach the client; raw repository prefixes and driver errors collapse to `-32603` with a server-side log. `SemanticError.Cause` (which can echo the raw date) is never serialized. | `internal/mcp/errors.go:41-72` |
| **PII** | Exception rows contain business calendar metadata and a free-text `reason` — **no client PII**. The audit record logs no caller id. | entity has no client fields; audit helper contract |
| **Privilege escalation** | The tools grant no role. There is no `action` parameter, no identifier rewriting (ids are not caller-derived), and the caller cannot select a role. | pattern of the 8 maintenance tools |
| **Loopback invariant** | Unchanged: the server still binds `127.0.0.1:3000` (`REQ-MT-001`). | — |
| **Schema integrity** | No schema change, so no migration attack surface and no data transformation. | D2/Non-goals |

**Defense in depth summary:** transport RBAC (`{owner}`) → use-case `RequireRole(RoleOwner)` → repository `RequireRole(admin, owner)` → SQL `UNIQUE(exception_date)`. Four barriers; the innermost two predate this change.

---

## 7. Testing approach

| Layer | Test focus |
|---|---|
| Use case (`internal/application/usecase`) | owner gate rejects admin/staff/client with `-32001`; create with closed/open shapes; duplicate → conflict Spanish; invalid times/date → invalid-input Spanish; delete unknown id → not-found Spanish; list range pass-through and empty result; audit emitted once per mutation (create/delete) and never for list; read-back returns the stored row with its id |
| Transport (`internal/mcp`) | `tools/list` includes the three names and the count is 23; input struct tags/required fields; output shapes; semantic errors pass through `errors.go` as `-32002`; only `*domain.SemanticError` reaches the client |
| Integration (real mux) | owner creates an exception → `check_availability` for that date changes (proves the read path honors the wire-created row); delete removes it and availability reverts; staff/client rejected on all three; count over the real mux is 23 |
| Repo | unchanged — its test suite already covers the guards and validations (`internal/repository/business_hours_exception_test.go`, `_roles_test.go`, `_dos_test.go`) |

Commands run at phase gates (not per work unit), per AGENTS.md: `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`. Focused tests first per work unit. `openspec/config.yaml` sets `apply.tdd: true`, so each work unit follows RED→GREEN→TRIANGULATE→REFACTOR.

---

## 8. Rollout / rollback

**Rollout order:** (1) use cases + DTOs (no external behavior change); (2) transport tools + ports + `ToolRBAC` + DI (this is the step that changes the surface); (3) `expectedToolCount` + README + base-spec reconciliation + integration tests.

**Rollback:** revert the tool registrations, `ToolRBAC` rows and DI; the surface returns to 20 tools. Repository, schema and read paths were never touched, and exception rows created through the tools remain valid and still govern availability. **No migration to reverse; no destructive operation in any step.**

**Observability of the change taking effect:** the audit record per mutation and the integration test asserting `check_availability` flips for the created date.

---

## 9. Open questions (owner) — all resolved (owner decision 2026-10-11)

Carried from `proposal.md`; each was a human decision, now settled.

| # | Question | Resolution (owner decision 2026-10-11) |
|---|---|---|
| OQ-1 | Tool count 20 → 21 (task file) vs 20 → 23 (three tools, D1) | **RESOLVED: accept 23** — one action per tool per ADR-0015 |
| OQ-2 | Capability delta directory name (plural, per the delegation memo) vs base capability `business-hours-exception` (singular) | **RESOLVED: singular** — delta dir renamed to `business-hours-exception` |
| OQ-3 | Expose `created_at` on the wire despite PRD §513's "sin timestamps" note | **RESOLVED: hidden from the wire** — drop it from `hoursExceptionOut`, mirroring `serviceOut` timestamps (PRD §513 convention) |
| OQ-4 | Tool name verb `list_hours_exceptions` (no `list_*` precedent) | **RESOLVED: keep** `list_hours_exceptions` (`get_hours_exceptions` rejected) |
| OQ-5 | Base transport spec reads 19 tools (stale vs code 20) | **RESOLVED: pre-step backport** — the archived whatsapp 19 → 20 delta was applied to `openspec/specs/mcp-transport/spec.md` |

---

## 10. Rejected alternatives

| Alternative | Why rejected |
|---|---|
| **One combined `manage_hours_exception` tool with an `action` discriminator** | No existing tool multiplexes actions; it breaks the one-action-per-tool precedent (`create_service`/`delete_service`/`upsert_schedule`/`delete_schedule`) and forces the LLM to build an action value. It is the only way to land the task-file's "20 → 21" (OQ-1). |
| **Add `Update` to the repository** | The repository documents exceptions as immutable and delete+create as the only edit path (`internal/repository/business_hours_exception.go:19-22`). It would also change the port and every conforming mock — outside this change's "no repository change" scope. |
| **Delete by `exception_date` instead of `id`** | The repo deletes by id (`:175`); delete-by-date requires a `Get` first and hides the row identity. The `id` is already exposed for schedules/services. |
| **List with an "upcoming N days" default** | Invents product semantics the PRD does not authorize; the repo's `List` requires both bounds, so mirroring it is the minimal contract (D4). |
| **Professional-scoped exceptions** | The table has no `professional_id` (`docs/PRD.md:266-274`); exceptions are business-wide. Adding scoping is a schema change and a product change. |
| **Reuse `update_business_profile` to carry the exception list** | `business_hours` is the weekly JSON; the PRD keeps exceptions in a separate table precisely to avoid mixing the two (`docs/PRD.md:259`, ADR-0006 Decision 2). |
| **Admin-TUI editor for exceptions** | ADR-0016 rejects duplicating the maintenance write surface in the TUI (`docs/architecture/0016-admin-tui-scope.md:58`, `:110`). |
| **Rate limiting the tools** | No anonymous reachability and an authenticated owner caller; no analogous threat to `register_client` (D6). |

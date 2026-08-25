# Exploration: feat-mcp-server-advanced

> Change: `feat-mcp-server-advanced` · Fase 3
> Phase: explore
> Reference: `docs/PRD.md` §3.7.9, §3.7.10, §3.8.7, §5.1 RF3 / RF7 / RF8
> Owner: mcp-appointments-crm
> Preflight (2026-08-24): interactive · both (hybrid) · ask-on-risk · 400 lines

## Current State

The MCP server (`cmd/mcp-server/main.go`) currently exposes **6 tools** wired through consumer ports in `internal/mcp/ports.go` and registered in `internal/mcp/server.go::registerTools` → `registerBookingTools` (5 booking tools) + `registerProfileTool` (1 profile tool). Composition root injects 6 use cases into `mcp.Config`, RBAC lives in `auth.ToolRBAC` (cmd/mcp-server/main.go:178-184) keyed on tool name.

**FTS5 infrastructure is already live** in the schema (`internal/db/schema.go::ftsTableDDL` / `ftsTriggerDDL`):

- `clients_fts` (FTS5, content=`clients`, indexed columns: `name`, `preferences`) — sync via 3 triggers (`clients_fts_ai/_ad/_au`).
- `services_fts` (FTS5, content=`services`, indexed columns: `name`, `description`) — sync via 3 triggers (`services_fts_ai/_ad/_au`).
- `schema_version` row pinned at `version=1` via `seedDDL()`; `initSchema` is idempotent (IF NOT EXISTS / INSERT OR IGNORE).
- Pragmas applied per-connection via `buildDSN` (`_pragma` query params): `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=on`, `synchronous=normal`.

**FTS repository methods already exist**:

- `ClientsRepo.SearchFTS(ctx, query) ([]*entity.Client, error)` (internal/repository/clients.go:310) — joins `clients c` with `clients_fts f` on `rowid`, orders by `bm25(f)`. **RBAC-aware** (calls `auth.RequireCaller` + `applyClientsAuthFilter`): clients see only their own client_id; admin/owner see all; staff is rejected (no role branch for staff → forbidden). FTS query validation via `validateFTSQuery` (rejects empty / FTS5 operator chars / whole-word AND/OR/NOT).
- `ServicesRepo.SearchFTS(ctx, query) ([]*entity.Service, error)` (internal/repository/services.go:143) — same JOIN + `bm25` order. **Auth-gated** to `auth.RoleAdmin | auth.RoleOwner` via `auth.RequireRole`. Validation through the same `validateFTSQuery`.
- `validateFTSQuery` (internal/repository/validation.go:62) — allowlist-based: `ftsQueryRe = [^\p{L}\p{N}\s\-]` rejects everything that is not Unicode letter/digit/whitespace/hyphen; `ftsOperatorRe` rejects whole-word AND/OR/NOT and leading +/-.

**Domain interfaces still expose the old (minimal) contracts**:

- `internal/domain/repository/clients.go::ClientsRepo` lists only `FindByID`, `FindByPhone`, `Save` — **`SearchFTS` is NOT in the interface yet**. The concrete repo has it; the domain interface must be widened before any use case can take a `ClientsRepo` dependency and reach `SearchFTS`.
- `internal/domain/repository/services.go::ServicesRepo` exposes only `FindByID`, `FindActive`, `Save`, `Update`, `Delete` — **`SearchFTS` is missing from the interface** as well.

**`pending_alerts` infrastructure is partially live**:

- Schema exists (internal/db/schema.go:110-119, 9 cells): columns `id`, `type`, `message`, `scheduled_datetime`, `status`, `related_booking_id`, `created_at`; index `idx_pending_alerts_scheduled_status` on `(scheduled_datetime, status)`; CHECK status in (pending/sent/cancelled).
- `PendingAlertsRepo` (internal/repository/pending_alerts.go) — 4 methods: `Save`, `FindPending(now)`, `MarkAsSent(id)`, `Cancel(id)`. All gated to `auth.RoleAdmin | auth.RoleOwner` (Save also rejects types outside the Fase 1 allowlist).
- `entity.PendingAlert` (internal/domain/entity/pending_alert.go) — entity with `IsDue`, `CanBeSent`, `IsValidType` (allowlist: `{"confirmation_requested": true}`).
- **PRD §3.7.9 documents three valid types**: `confirmation_requested`, `reminder_24h`, `loyalty_alert`. The current `allowedAlertTypes` map only contains `confirmation_requested`. **`reminder_24h` and `loyalty_alert` are missing from the entity allowlist** (this is a real gap when alert generation starts using them).
- The `domain.PendingAlertsRepo` interface **already exists** in `internal/domain/repository/pending_alerts.go` and matches the concrete repo 1:1.

**Alert generation is NOT implemented**. The flow described in PRD §3.7.13 Paso 5 (`INSERT INTO pending_alerts … VALUES ('confirmation_requested', 'Confirmar reserva de {client_name} con {pro_name} el {start_datetime}', …, ?, …)`) is documented but **not coded**. Neither `CreateBookingUseCase`, `CancelBookingUseCase`, nor `RescheduleBookingUseCase` calls into `PendingAlertsRepo.Save`. The 24h-reminder and loyalty-alert flows are also absent.

**Loyalty report is NOT implemented**. There is no `BookingsRepo.AggregateByClient`, no SQL with `GROUP BY client_id`, no `LoyaltyReportUseCase`, no `period` parser, no `top_n` parameter.

**Booking-related existing code (kept intact)**:

- `BookingsRepo.Create` (internal/repository/bookings.go:85) uses the atomic `INSERT … WHERE NOT EXISTS (overlap subquery)` pattern (design Decisión 11). Overlap excludes `status='cancelled'`. RowsAffected==0 → `domain.ErrConflict`.
- `BookingsRepo.Cancel/Reschedule` plus the **disambiguation pattern** (`SELECT status FROM bookings WHERE id=?` after `UPDATE` with `rowsAffected==0` to distinguish overlap vs cancelled vs not-found) is implemented in internal/repository/bookings.go (~lines 407-430).
- `BookingStatus` FSM in `entity/booking.go`: pending → {confirmed, cancelled}; confirmed → cancelled; cancelled → terminal. Transitions enforced via `IsValidTransition` / `CanTransitionTo`.
- `auth.AuthorizeBookingAccess(caller, booking)` — cross-tenant isolation: client↔own booking, staff↔own professional, admin/owner any.

**RBAC and auth model**:

- 4 roles: `owner`, `admin`, `staff`, `client`. `owner` and `admin` are operationally equivalent (PRD §3.8.4).
- `auth.ToolRBAC` map in cmd/mcp-server/main.go:178-184 already lists 5 tools; `check_availability` has no entry (open to any authenticated caller).
- `Caller` is resolved by `auth.CallerResolver` (≤2 queries: accounts, then optionally clients). ID == phone for client / professional_id-bearing staff; `auth.Caller.ClientID` is the UUID PK into `clients`.
- `applyClientsAuthFilter` (internal/repository/clients.go:34) is the established pattern for per-row tenant scoping. Clients get `AND id = ?`; admin/owner get no filter; staff gets forbidden (no branch).

**Existing use case pattern (the new use cases must follow it)**:

- File: `internal/application/usecase/<name>.go`. Struct + `New<Type>UseCase(...)` constructor + `Execute(ctx, dto.<Name>Input) (*dto.<Name>Result, error)`.
- First line of `Execute`: `auth.RequireAuthenticated(input.Caller)` (or `auth.RequireRole(ctx, auth.RoleX, ...)` for the read tools that are admin/owner-only).
- DTO file: `internal/application/dto/<name>.go` with `Caller auth.Caller \`json:"-"\`` field + tool-specific args + `Result` struct with `json:` tags (snake_case for output, e.g., `BookingView`).
- Consumer-side port: `internal/mcp/ports.go` — `interface { Execute(context.Context, dto.<Name>Input) (*dto.<Name>Result, error) }`. Test `TestNoRepositoryImport` enforces that `internal/mcp/` never imports `internal/repository/`.
- Tool handler in `internal/mcp/tools_<area>.go`: `mcp.AddTool(s.impl, s.mcpTool(name, desc), func(ctx, _ *mcp.CallToolRequest, in <input>) (...) { caller, _ := auth.RequireCaller(ctx); res, err := s.cfg.<Port>.Execute(ctx, dto.<Name>Input{...}); ... })` + register name into `s.toolNames[name] = struct{}{}`.
- Tool RBAC entry added to the `auth.ToolRBAC` map (cmd/mcp-server/main.go).
- Composition root: construct use case, add to `mcp.Config{...}`, and (if needed) add a new `internal/mcp.Config` field with the corresponding consumer port.

**Test coverage of the touched layers is strong**: repository tests use real SQLite (in-memory `_test.go` files: `clients_test.go` 15.9 KB, `business_hours_exception_test.go` 14 KB, `business_hours_exception_roles_test.go` 4 KB, `clients_role_test.go` 3.7 KB, `clients_scope_test.go` 5.6 KB, `pending_alerts_test.go` 5.2 KB, `validation_test.go`); use case tests use function-table mocks (mocks_test.go 11.8 KB covers all `repository.*Repo` interfaces). The FTS5 sync integration test and end-to-end booking flow are already covered; existing tools have handler-level tests in `internal/mcp/tools_test.go` and integration tests in `internal/mcp/server_integration_test.go`.

## Affected Areas

- `internal/domain/repository/clients.go` — **add `SearchFTS` to the `ClientsRepo` interface** so use cases can call it through the abstraction. Currently only `FindByID` / `FindByPhone` / `Save` are exposed; widening the interface is the contract change.
- `internal/domain/repository/services.go` — **add `SearchFTS` to the `ServicesRepo` interface** (mirrors clients).
- `internal/repository/clients.go` — already has `SearchFTS`; ensure it remains RBAC-aware (calls `RequireCaller` + `applyClientsAuthFilter`). Validate that staff role returns `ErrForbidden` (currently no branch → forbidden; PRD §3.8.4 keeps this consistent).
- `internal/repository/services.go` — already has `SearchFTS`; admin/owner gating via `RequireRole` is correct (services are not per-tenant; cross-staff data is fine for the trusted roles).
- `internal/repository/validation.go` — `validateFTSQuery` + `ftsQueryRe` + `ftsOperatorRe` are already battle-tested; reuse without modification.
- `internal/domain/entity/pending_alert.go` — **extend `allowedAlertTypes`** from `{confirmation_requested: true}` to include `reminder_24h` and `loyalty_alert` (matches PRD §3.7.9).
- `internal/domain/repository/pending_alerts.go` — interface already exists; **add `FindByBookingID` or equivalent** so cancel/reschedule flows can cancel pending alerts tied to a booking (see "Approaches" §2 below). Also possibly a `ListScheduledBetween(from, to)` for the 24h-reminder query.
- `internal/repository/pending_alerts.go` — concrete impl may need a new `FindByBookingID` (UPDATE by `related_booking_id AND status='pending'` to cancel pending alerts when the booking is cancelled) and `MarkAsSent` / `Cancel` already exist idempotently.
- `internal/application/usecase/create_booking.go` — **insert a `confirmation_requested` alert** after the atomic INSERT succeeds. Message format from PRD §3.7.13 Paso 5: `"Confirmar reserva de {client_name} con {pro_name} el {start_datetime}"`. Add a deferred `BookingsRepo.FindByID` (or carry the `Booking` struct from the validator) and `ClientsRepo.FindByID` + `ProfessionalsRepo.FindByID` to resolve names. **Idempotency decision**: store the booking id once and accept that retries on the same booking create duplicate alerts; alternatively key alerts by `(related_booking_id, type)` UNIQUE.
- `internal/application/usecase/cancel_booking.go` — **cancel pending alerts** for the booking (use `PendingAlertsRepo.CancelByBookingID` or `UPDATE pending_alerts SET status='cancelled' WHERE related_booking_id=? AND status='pending'`). Idempotent (matches existing `Cancel` semantics).
- `internal/application/usecase/reschedule_booking.go` — **cancel pending alerts and insert a new `confirmation_requested`** with the new `start_datetime`. Same RBAC-aware client/professional resolution as create.
- `internal/application/usecase/search_clients_advanced.go` (new) — `SearchClientsAdvancedUseCase` calling `ClientsRepo.SearchFTS(ctx, query)`. Auth: `RequireCaller` (delegated to repo filter). Returns `[]dto.ClientView` with snake_case JSON tags.
- `internal/application/usecase/search_services_advanced.go` (new) — `SearchServicesAdvancedUseCase` calling `ServicesRepo.SearchFTS`. Auth: `RequireRole(ctx, RoleAdmin, RoleOwner)`. Returns `[]dto.ServiceView`.
- `internal/application/usecase/get_pending_alerts.go` (new) — `GetPendingAlertsUseCase` calling `PendingAlertsRepo.FindPending(ctx, time.Now().UTC())`. Auth: `RequireRole(ctx, RoleAdmin, RoleOwner)`. Returns `[]dto.PendingAlertView`.
- `internal/application/usecase/mark_alert_as_sent.go` (new) — `MarkAlertAsSentUseCase` calling `PendingAlertsRepo.MarkAsSent(ctx, id)`. Auth: `RequireRole(ctx, RoleAdmin, RoleOwner)`. Returns `dto.MarkAlertAsSentResult{AlertID, Status}`.
- `internal/application/usecase/get_loyalty_report.go` (new) — `GetLoyaltyReportUseCase` with a period parser (`"last_week" | "last_month" | "last_quarter" | "last_year" | "all_time"`, default `last_month`), `top_n` (default 10, clamped to e.g. 1..100). SQL aggregation lives in repo (see Approaches §3). Auth: `RequireRole(ctx, RoleAdmin, RoleOwner)`. Returns `[]dto.LoyaltyReportEntry{ClientID, ClientName, Phone, BookingCount}`.
- `internal/repository/bookings.go` — **add a new aggregation method** `AggregateBookingsByClient(ctx, from, to time.Time) ([]ClientBookingCount, error)` (or split into a new repo if cleaner) — see Approaches §3.
- `internal/domain/dto/` (new DTOs) — `search_clients_advanced.go`, `search_services_advanced.go`, `get_pending_alerts.go`, `mark_alert_as_sent.go`, `get_loyalty_report.go`. Each follows the existing `<input>Input`/`<input>Result` convention with `Caller auth.Caller \`json:"-"\``.
- `internal/mcp/ports.go` — add 5 new ports: `SearchClientsAdvancedPort`, `SearchServicesAdvancedPort`, `GetPendingAlertsPort`, `MarkAlertAsSentPort`, `GetLoyaltyReportPort`.
- `internal/mcp/config.go` — add 5 fields to `Config` (mirrors the new ports).
- `internal/mcp/tools_search.go` (new) — `registerSearchTools` wiring `search_clients_advanced` and `search_services_advanced`. Follow `internal/mcp/tools_booking.go` pattern.
- `internal/mcp/tools_alerts.go` (new) — `registerAlertTools` wiring `get_pending_alerts` and `mark_alert_as_sent`.
- `internal/mcp/tools_reports.go` (new) — `registerReportTools` wiring `get_loyalty_report`.
- `internal/mcp/server.go` — extend `registerTools` to call the three new registrars (still optional ports: a nil port leaves its tool unregistered, preserving transport tests).
- `cmd/mcp-server/main.go` — construct the 5 new use cases, add to `mcp.Config`, extend `auth.ToolRBAC`:
  - `search_clients_advanced`: any authenticated caller (clients restricted to own id via the repo filter; staff currently forbidden — needs a design decision; admin/owner any).
  - `search_services_advanced`: owner + admin (services aren't per-tenant, matches existing `FindActive` policy).
  - `get_pending_alerts`: owner + admin (PRD RF7 says Hermes consumes; the channel is the trusted MCP client behind loopback, so admin/owner gate keeps the audit trail clean).
  - `mark_alert_as_sent`: owner + admin.
  - `get_loyalty_report`: owner + admin (PII: client phones + booking counts).
- `internal/db/schema.go` — **add a 2nd row to `seedDDL()` for `schema_version` row with `version=2`** describing the alert-generation addition. Or keep `schema_version=1` and only bump on destructive migrations (Fase 3 is additive: no DDL change to schema because `pending_alerts` and the FTS tables already exist).
- `internal/db/database.go::initSchema` — **no changes**; schema is already additive.
- `docs/PRD.md` — confirm RF3/RF7/RF8 acceptance criteria against the chosen approach; bump to v1.10 with Fase 3 closure (status block) once implementation lands.

## Approaches

### 1. FTS query sanitization strategy (RF3)

1. **A1: Existing `validateFTSQuery` (status quo) — allowlist + reject** (✅ recommended)
   - Already implemented in `internal/repository/validation.go::validateFTSQuery`. Two regexes (`ftsQueryRe` + `ftsOperatorRe`) reject FTS5 operators and most punctuation while preserving Unicode letters / digits / whitespace / hyphens.
   - Pros: zero new code; Spanish accented terms (`alergía`, `María`) work; the rules are unit-tested in `internal/repository/validation_test.go`; pragmatic — refuses queries that would otherwise need per-call escaping.
   - Cons: can't pass true FTS5 boolean queries (`"geo AND local"`, `"barber -peluquería"`). For an LLM-driven `search_*` tool that's a feature, not a bug.
   - Effort: **Low** — reuse as-is.

2. **A2: Pass-through to FTS5 with quoted-term escaping**
   - Wrap each whitespace-separated token in double quotes; double any embedded quote; refuse tokens containing `*`.
   - Pros: allows power users to combine multiple terms.
   - Cons: silently changes semantics (the query `"foo bar"` matches the phrase `foo bar`, not docs containing both); the LLM would need to learn quoted FTS5 syntax; escapes are error-prone (one missed quote = SQL error or wrong results); adds a second helper that needs its own test surface.
   - Effort: **Medium** — escape helper + tests, plus PRD criterion review (RF3 says "escapa los caracteres y retorna resultados válidos o un mensaje semántico claro, nunca un error de SQL" — A2 satisfies the spirit, A1 satisfies the letter).

### 2. Alert generation placement — where the `pending_alerts` INSERT lives

The three booking mutations (create / cancel / reschedule) must each interact with `pending_alerts`. Three placements are possible.

1. **B1: Use case layer (✅ recommended)**
   - After the atomic repo mutation succeeds, the use case resolves the booking / client / professional names and calls `PendingAlertsRepo.Save` (or `Cancel`).
   - Pros: keeps the repository pure data-access (no surprise side effects in the SQL); matches the existing layering (use case orchestrates, repo persists); the alert text template lives next to the business rule that decides when to emit it; tests of the use case naturally cover alert emission.
   - Cons: the use case constructors widen (each booking use case gains `PendingAlertsRepo` and the read repos needed for name resolution — `ClientsRepo` / `ProfessionalsRepo` for create; existing `bookings` repo for cancel/reschedule already has the booking entity).
   - Effort: **Low/Medium** — only the 3 booking use cases change.

2. **B2: Domain service (`BookingAlertService`)** — separate service that owns the alert-emission policy
   - `BookingAlertService.OnBookingCreated(ctx, booking)`, `.OnBookingCancelled(ctx, bookingID)`, `.OnBookingRescheduled(ctx, booking, newStart)`.
   - Pros: clean separation of concerns; testable in isolation; future triggers (status change to confirmed → no alert, but status change to cancelled outside of cancel_booking → still alert) live in one place.
   - Cons: yet another package surface; another constructor; over-engineering for 3 call sites.
   - Effort: **Medium/High**.

3. **B3: Inside the repository (SQL trigger)** — a SQLite `AFTER INSERT ON bookings` trigger inserts the pending_alert row automatically
   - Pros: zero Go code; impossible to forget.
   - Cons: needs the client/professional names inside the trigger — possible via `SELECT name FROM clients WHERE id = NEW.client_id` but reads (a) duplicate the alert text template across Go/SQL, (b) make it harder to test in Go, (c) make it harder to evolve (Fase 3.5+ may add `reminder_24h` — every new alert type needs new triggers); also a `cancel_booking` trigger would need to `UPDATE pending_alerts`, which is harder to scope per-row. **Rejects** the project's spirit of "business logic in Go, SQL is the data layer".
   - Effort: **Low** SQL-wise, but **High** overall because of testing and template duplication.

### 3. Loyalty report aggregation — SQL vs app-layer

1. **C1: Pure SQL `GROUP BY` with `COUNT(*)` and `JOIN clients` (✅ recommended)**
   - One query:
     ```sql
     SELECT c.id, c.name, c.phone, COUNT(b.id) AS booking_count
     FROM clients c
     JOIN bookings b ON b.client_id = c.id
     WHERE b.status != 'cancelled'
       AND b.start_datetime >= ?  -- period start (UTC)
       AND b.start_datetime <  ?  -- period end (UTC)
     GROUP BY c.id
     HAVING booking_count > 0
     ORDER BY booking_count DESC, c.name ASC
     LIMIT ?;                      -- top_n
     ```
   - Pros: single round-trip; the index `idx_bookings_overlap` on `(professional_id, start_datetime, end_datetime)` does not cover this query but a SQLite planner with `WHERE b.start_datetime BETWEEN ? AND ?` is fast on a small dataset (single-tenant CRM); PRD NNF target is p95 < 100 ms which the indexed seek easily meets for any realistic tenant.
   - Cons: requires either adding `BookingsRepo.AggregateByClient` (which mixes concerns) or creating a new `ReportsRepo` interface + impl (cleaner).
   - Effort: **Low** — single repo method + use case + tool.

2. **C2: App-layer aggregation**
   - `BookingsRepo.ListBookingsForRange` already exists; the use case loads all bookings, groups in Go, counts per client.
   - Pros: no new SQL; portable across DB engines.
   - Cons: pulls every booking in the period into memory; the overlap index helps the range query but the N is the total booking count for the period — for a busy salon this is millions of rows over "last_year"; unacceptable.
   - Effort: **Low** code-wise, **High** runtime risk.

3. **C3: Materialized view / precomputed summary table**
   - Pros: O(1) reads for the report.
   - Cons: needs maintenance triggers; SQLite has no native materialized views; massive over-engineering for an MCP tool that's called once a day per tenant.
   - Effort: **High** — design + maintenance burden > value.

### 4. Alert idempotency on create_booking retries

1. **D1 (✅ recommended): Trust the booking id as the alert dedup key; accept one alert per (booking_id, type).**
   - Add UNIQUE index on `(related_booking_id, type)` for pending_alerts; the second INSERT raises `ErrConflict`; the use case logs and continues (this is a recovery from a partial failure, not an error).
   - Pros: simple; aligns with the existing `MarkAsSent`/`Cancel` idempotency model.
   - Cons: requires a schema migration (UNIQUE index) and a `INSERT OR IGNORE` (or `ON CONFLICT`) — both fine in SQLite.

2. **D2: No dedup; let duplicates stack**
   - Simpler; doesn't require the index. If Hermes retries the booking, the operator sees N confirmation alerts.
   - Cons: UX risk (Hermes re-confirms with the client N times).

3. **D3: Use an in-memory dedup map keyed on booking_id within the process**
   - Useless across restarts.

### 5. Test posture for FTS5 / alerts (refines existing convention)

- Repository tests: SQLite in-memory (`internal/repository/*_test.go`), covering `SearchFTS` ranking, empty-result, and operator rejection. Existing `clients_test.go` (15.9 KB) and `services_test.go` already have the harness; add cases for the new auth paths (admin sees all, client sees only own, staff forbidden).
- Use case tests: function-table mocks (`internal/application/usecase/mocks_test.go` pattern, 11.8 KB) — add `mockPendingAlertsRepo` and inject into the booking use cases' mocks.
- End-to-end: `internal/mcp/server_integration_test.go` (11 KB) — assert the 5 new tools appear in `tools/list` and the alert is inserted on a real `create_booking` call against a real SQLite DB.
- Alert cancel-on-cancel: integration test that creates a booking, cancels it, then asserts `pending_alerts` has zero rows with `status='pending'` for that `related_booking_id`.

## Recommendation

Adopt **A1 + B1 + C1 + D1**:

- **A1**: reuse `validateFTSQuery` as the FTS query sanitizer. The PRD criterion "escapa los caracteres y retorna resultados válidos o un mensaje semántico claro, nunca un error de SQL" is met by the existing helper that returns `ErrInvalidInput` (mapped to `codeBusinessError = -32002` with the Spanish message).
- **B1**: insert/cancel alerts from the use case layer. The 3 booking use cases (`CreateBookingUseCase`, `CancelBookingUseCase`, `RescheduleBookingUseCase`) gain a `PendingAlertsRepo` dependency plus the read repos needed for name resolution. The alert text template is colocated with the booking-mutation policy that triggers it.
- **C1**: pure SQL `GROUP BY` aggregation in a new method on `BookingsRepo` (or a small `ReportsRepo` if separation is preferred — see note below). Keep app-layer logic to the period parser and `top_n` clamp.
- **D1**: UNIQUE index on `(related_booking_id, type)` to dedup alerts; `INSERT OR IGNORE` in the use case; log the duplicate but don't fail.

**Open question for `sdd-propose`**:

- The `search_clients_advanced` RBAC: PRD §3.8.4 doesn't say staff can FTS-search clients. Current `applyClientsAuthFilter` rejects staff. The cleanest posture is to keep staff out of the FTS endpoint and document it explicitly in the spec ("staff only sees their own clients via `get_client_history`; FTS search is admin/owner/client-self only").
- Aggregate method location: `BookingsRepo.AggregateByClient(from, to, limit) ([]ClientBookingCount, error)` keeps everything in one repo and matches the existing `ListBookingsForRange` style. A separate `ReportsRepo` is cleaner architecturally but adds a port + constructor + wire-up for a single method. **Recommend**: keep on `BookingsRepo`.

## Risks

- **FTS5 operator handling edge cases**: the current `ftsQueryRe` rejects characters like `?`, `:`, `(`, `)`. The PRD explicitly says the system must handle these gracefully ("nunca un error de SQL"). The existing `validateFTSQuery` does, but the message — "la consulta contiene caracteres no permitidos" — may confuse an LLM that didn't expect it. **Mitigation**: tests for 4-5 representative edge cases (paréntesis, comillas, asterisco, +, -); document the behavior in the tool description in Spanish.
- **`reminder_24h` allowlist mismatch**: `entity.PendingAlert.allowedAlertTypes` only contains `confirmation_requested`. PRD §3.7.9 lists three types. **Risk**: alerting needs the allowlist expanded BEFORE Fase 3 can claim RF7 complete. Tracked in "Affected Areas" §6 above.
- **Alert emission failures vs booking success**: if `bookings.Create` succeeds but `pending_alerts.Save` fails, the booking is committed and the alert is lost. **Mitigation**: the use case logs the alert-save error with `slog.Warn` (operator can re-emit manually) and still returns success — alternative is to wrap both in a SQLite transaction, but the existing repo methods don't expose `*sql.Tx`; transactional alert+booking is a refactor that should be deferred (separate observation, no scope creep).
- **Cancel-on-cancel cascading**: cancelling a booking should also cancel its pending alerts. If the new method `PendingAlertsRepo.CancelByBookingID` forgets to filter `status='pending'`, already-sent alerts flip to `cancelled` — confusing for audits. **Mitigation**: idempotent filter enforced in SQL (`AND status='pending'`) + integration test.
- **Loyalty report PII leak**: returns `phone` per row. PRD RF8 says so, but if a future export hits an LLM with the report in context, the LLM may echo phones back. **Mitigation**: RBAC keeps the endpoint to owner/admin (loopback trust); document the PII surface in the spec.
- **`top_n` clamp drift**: a malicious or buggy LLM could request `top_n=1000000`. **Mitigation**: clamp to e.g. `[1, 100]` and return `ErrInvalidInput` outside that range.
- **Schema-version bump**: Fase 3 is purely additive (no DDL change to `pending_alerts` or FTS tables), so `schema_version=1` stays valid. The UNIQUE index proposed in §D1 IS a schema change — option D2 (no dedup) avoids it; option D1 requires bumping to `schema_version=2` in `seedDDL()` (only the seed row count grows, no DDL changes otherwise because `CREATE UNIQUE INDEX IF NOT EXISTS` is idempotent inside `initSchema`).
- **400-line PR budget**: Fase 3 spans ~5 new use cases + 5 new DTOs + 5 new MCP tools + alert-emission wiring in 3 existing use cases + repo widening + new test files. This is a clear **High** budget risk. **Mitigation**: plan chained PRs per the Verification & Review Protocol §E — sensible slices are (a) FTS interfaces + use cases + tools + tests, (b) alerts repository expansion + use cases + tools + integration tests, (c) loyalty report + tool + tests, (d) wire alert emission into create/cancel/reschedule (cross-cutting, smallest PR but highest blast radius).

## Ready for Proposal

**Yes** — the exploration is complete. The orchestrator can move to `sdd-propose` with the following handoff:

- Approach stack: **A1 + B1 + C1 + D1** (and the open question on staff-RBAC for `search_clients_advanced` should be settled before the proposal).
- Schema posture: **additive only** — no DDL changes to existing tables. If D1 is adopted, add `CREATE UNIQUE INDEX IF NOT EXISTS uniq_pending_alerts_booking_type ON pending_alerts(related_booking_id, type) WHERE related_booking_id IS NOT NULL` and bump `schema_version` to `2`.
- Five new MCP tools: `search_clients_advanced`, `search_services_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`.
- Three booking use cases (create/cancel/reschedule) gain a `PendingAlertsRepo` dependency + name-resolution repos.
- Domain interfaces widened: `ClientsRepo` / `ServicesRepo` get `SearchFTS`; `PendingAlertsRepo` gets `CancelByBookingID`.
- The orchestrator should ask the user: "Confirmamos los 4 enfoques A1+B1+C1+D1 y dejamos D2 (sin dedup) como fallback si la unique index complica el seed?"
- Then proceed to **sdd-propose** (intent, scope, approach, rollback plan, with the chained-PR plan referenced explicitly because the budget is clearly High).

**Estimated budget impact**: clearly > 400 lines if shipped as a single PR. Recommend 3-4 chained PRs aligned with the slicing in Risks §R-7. The orchestrator must set `delivery_strategy` to `chained` (or get explicit `size:exception` acceptance) BEFORE running `sdd-tasks`.
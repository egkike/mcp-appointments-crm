# Exploration: feat-repository-auth-integration

> Change: `feat-repository-auth-integration`
> Phase: explore
> Reference: `docs/PRD.md` §3.8.7 ("Deuda de auth (actualizada)")
> Owner: mcp-appointments-crm

## Current State

`auth.Caller` is wired into most repositories of PR 3a (`internal/repository/bookings.go`, `professionals.go`, `schedules.go`, `pending_alerts.go`) and into PR 2's `services.go` (read paths open to all authenticated callers; writes gated by `auth.RequireRole(RoleAdmin, RoleOwner)`). Two PR 2 repositories remain without any `auth.Caller` integration: **`clients.go`** and **`business_hours_exception.go`**. Both still pass `ctx` straight through to `ExecContext`/`QueryContext` without consulting `auth.FromContext` / `auth.RequireCaller` / `auth.RequireRole` / `applyAuthFilter`.

PRD §3.8.7 (line 853, dated 2026-08-20) calls this out as the last remaining auth integration debt, prerequisite to Fase 3 (mcp-server-advanced). Phase 2 (mcp-server-core, `feat-mcp-transport`) did NOT depend on this wiring because every consumer for those two tables is a use case that already enforces its own role check (`CreateBookingUseCase` and `RescheduleBookingUseCase` route through `service.ResolveSlotContext` → `BusinessHoursExceptionRepo.Get`; no MCP tool of Fase 2 touches `ClientsRepo`).

### Affected repositories

#### `internal/repository/clients.go` (218 lines)

Methods (lines reference `internal/repository/clients.go`):

| Method | Lines | Current SQL | Auth today |
|---|---|---|---|
| `Save(ctx, c)` | 31–47 | `INSERT OR REPLACE INTO clients ...` | none |
| `Create(ctx, c)` | 51–70 | `INSERT INTO clients ...` | none |
| `FindByID(ctx, id)` | 73–87 | `SELECT ... FROM clients WHERE id = ?` | none |
| `FindByPhone(ctx, phone)` | 90–104 | `SELECT ... FROM clients WHERE phone = ?` | none |
| `GetOrCreate(ctx, phone, name)` | 108–133 | `INSERT OR IGNORE` + `SELECT ... WHERE phone = ?` | none |
| `Update(ctx, c)` | 138–165 | `UPDATE clients SET ... WHERE id = ?` | none |
| `Delete(ctx, id)` | 168–181 | `DELETE FROM clients WHERE id = ?` | none |
| `SearchFTS(ctx, query)` | 186–217 | `SELECT ... JOIN clients_fts f ON c.rowid = f.rowid WHERE f MATCH ? ORDER BY bm25(f)` | none |

Compile-time interface check at line 17: `var _ domainrepo.ClientsRepo = (*ClientsRepo)(nil)`. Domain interface at `internal/domain/repository/clients.go:11–20` exposes only `FindByID`, `FindByPhone`, `Save` — the rest (`Create`, `GetOrCreate`, `Update`, `Delete`, `SearchFTS`) are extra concrete methods not part of the published contract.

Schema (PRD §3.7.10, PRD.md:339–348): `id TEXT PK`, `name TEXT`, `phone TEXT UNIQUE`, `email TEXT`, `preferences TEXT`, `created_at`, `updated_at`. There is **no `client_id` column** on `clients`; the row's own `id` IS the client id (entity.Client exposes `ID`).

#### `internal/repository/business_hours_exception.go` (153 lines)

Methods (lines reference `internal/repository/business_hours_exception.go`):

| Method | Lines | Current SQL | Auth today |
|---|---|---|---|
| `Create(ctx, ex)` | 38–84 | `INSERT INTO business_hours_exception ...` | none |
| `Get(ctx, date)` | 89–104 | `SELECT ... WHERE exception_date = ?` | none |
| `List(ctx, from, to)` | 108–136 | `SELECT ... WHERE exception_date >= ? AND exception_date <= ? ORDER BY exception_date` | none |
| `Delete(ctx, id)` | 139–152 | `DELETE FROM business_hours_exception WHERE id = ?` | none |

Compile-time check at line 16. Domain interface at `internal/domain/repository/business_hours_exception.go:12–25`: `Get`, `Create`, `List`, `Delete`. There is no `Update` (exceptions are immutable by design — repo comment line 21).

Schema (PRD.md:256–268): `id INTEGER PK AUTOINCREMENT`, `exception_date TEXT UNIQUE`, `is_closed BOOLEAN`, `open_time TEXT`, `close_time TEXT`, `reason TEXT`, `created_at TEXT`. No `client_id` or `professional_id` column.

### Canonical patterns to reuse

#### Pattern A — `auth.RequireRole` writes gate (already in `services.go`)

`internal/repository/services.go` (`Save`:32, `Update`:95, `Delete`:123):

```go
if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
    return fmt.Errorf("crear servicio: %w", err)
}
```

Use case: pure write paths — no row-scoping needed because only admins/owners can hit them.

#### Pattern B — `applyAuthFilter` scoped reads (already in `bookings.go`)

`internal/repository/auth_filter.go` is a package-private helper (lines 21–99). For each caller's role it appends ` AND client_id = ?` (client) or ` AND professional_id = ?` (staff), and leaves the query unchanged for admin/owner. Repos that adopt it follow this template (`bookings.go:129–137`):

```go
caller, err := auth.RequireCaller(ctx)
if err != nil { return nil, err }
query := `SELECT ... FROM bookings WHERE id = ?`
args := []any{id}
query, args, err = applyAuthFilter(caller, query, args)
if err != nil { return nil, err }
row := r.db.QueryRowContext(ctx, query, args...)
```

Insertion logic uses `strings.LastIndex` on the uppercased query to find the last `ORDER BY` or `LIMIT`, then injects the filter clause before that boundary (auth_filter.go:79–96). Defensive copy of `args` (line 32).

### Why `applyAuthFilter` cannot be reused verbatim for `clients`

The bookings pattern fits a table with both a `client_id` and a `professional_id` column. The `clients` table has only `id` (the PK). For a `client` caller, scoping by `id = caller.ClientID` is correct, but `applyAuthFilter` always emits the literal `AND client_id = ?` (auth_filter.go:47) — wrong column for `clients`. For a `staff` caller, there is no staff-to-client relationship in the schema; staff callers should not access clients at all, so the appropriate wiring is `auth.RequireRole(ctx, RoleAdmin, RoleOwner)` (Pattern A), NOT a column-rewrite of `applyAuthFilter`. The cleanest design is a per-repo local helper that mirrors `applyAuthFilter`'s SQL-injection strategy but uses `AND id = ?` for the `clients` case. Equivalently, `bookings.go`'s `applyAuthFilter` could be parametrized to accept the filter column name, but that complicates the helper and is out of scope for this change (no current consumer needs the new column).

### PII / oracle risks

| Surface | Risk |
|---|---|
| `ClientsRepo.SearchFTS` | The `clients_fts` virtual table indexes `name` and `preferences` (spec `clients/spec.md:55–57`). Returning every client's `preferences` (free text including medical notes like `"alergia a penicilina"`) to any authenticated caller is a PII leak — clients should see only their own `preferences`, staff and admins should see only what their role permits. |
| `ClientsRepo.FindByPhone` | Phone is a UNIQUE identifier (PRD.md:342). If left unguarded, an attacker who knows or guesses a phone can enumerate which phones are in the system (existence oracle) and read the full row (including `preferences`). Must be admin/owner-only OR self via `id`. |
| `ClientsRepo.GetOrCreate` | Idempotent upsert by phone. Used by the bots. Must NOT be callable by a `client` caller against a foreign phone (would let them bind another client's `id` to a phone they control). Reasonable gate: `client` caller can only `GetOrCreate` for `phone == caller.ID`; admin/owner unrestricted; staff should not upsert. |
| `BusinessHoursExceptionRepo.Get` | Called from `service.ResolveSlotContext` and `service.AvailabilityService.CheckAvailability` — both are invoked on the hot path of `check_availability` and `create_booking`. ANY authenticated caller needs to read the exception for the slot they're booking. So `Get` must be open to all callers (it returns one row keyed by an immutable calendar date with no PII). |
| `BusinessHoursExceptionRepo.List` | Same data shape as `Get` (one row per date). Reasonable gate: open to all authenticated callers (used to render calendar views). No `client_id`/`professional_id` filter applicable. |
| `BusinessHoursExceptionRepo.Create` / `Delete` | This is the most dangerous one. The exception row gates "is the business open on date X?". Any caller who can insert a row dated `9999-12-31` with `is_closed=1` will permanently close the business from that LLM's view. Today there is NO auth gate, so any reachable caller could DoS bookings by planting holidays. **Must** be `auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner)` — no other role can write or delete. |

### Consumers today

Confirmed via repo: only **`bizHoursExRepo`** is wired in `cmd/mcp-server/main.go:121` and it's consumed by `CreateBookingUseCase` (line 50–59) and `RescheduleBookingUseCase` (line 41–50) through `service.ResolveSlotContext`. **`ClientsRepo` is currently not instantiated in `main.go`** at all — confirmed by `grep -n "ClientsRepo\\|NewClientsRepo" cmd/mcp-server/main.go` returning zero hits. The wiring change is repo-layer hardening (defense in depth) ahead of Fase 3 tools; it does NOT alter behavior of Fase 2 tools because Phase 2 doesn't expose clients or business-hours-exception as MCP operations.

Test harnesses (`internal/repository/testutil_test.go:42–67`) already expose `adminCtx`, `ownerCtx`, `staffCtx(profID)`, `clientCtx(clientID)`. Every existing role-aware test in `services_test.go` and `bookings_test.go` follows the pattern of substituting `context.Background()` with one of those helpers — clients_test.go and business_hours_exception_test.go will adopt the same pattern after wiring, and the test surface grows by approximately 9 cases per `RequireRole`-gated method (no-caller, each role accepted/rejected) plus per-role SQL filter cases for scoped methods.

## Affected Areas

- `internal/repository/clients.go` — wire `RequireCaller`/`RequireRole`/`applyAuthFilter`-style scoping into all 8 public methods; introduces a local helper for `AND id = ?` (client scope) because the canonical `applyAuthFilter` emits `AND client_id = ?`.
- `internal/repository/business_hours_exception.go` — wire `RequireRole(RoleAdmin, RoleOwner)` for `Create`/`Delete`; leave `Get`/`List` open to all authenticated callers (or wrap with `RequireCaller` only).
- `internal/repository/clients_test.go` — switch all `context.Background()` to a role-helper ctx where the method requires it; add role-mismatch cases (no caller / forbidden role).
- `internal/repository/business_hours_exception_test.go` — same transition; add no-caller + non-admin-rejected cases for `Create` and `Delete`.
- `internal/repository/auth_filter.go` — likely unchanged; the per-method scoping for `clients` (column `id` vs `client_id`) goes in a local helper inside `clients.go`, not in the shared file.
- `cmd/mcp-server/main.go` — no behavioral change required (current callers go through use cases; `ClientsRepo` not instantiated today). If the design later wants to surface admin CRUD through the TUI menu (PRD §3.8.8), the wiring happens there.
- `docs/PRD.md` §3.8.7 — once archived, the line about `clients.go` + `business_hours_exception.go` pending should flip to ✅; section should also drop the note that it's a "Fase 3 prerequisite" once archived.

## Approaches

### 1. Mirrored helper in `clients.go` (recommended)

Reuse `applyAuthFilter`'s shape but emit `AND id = ?` for the client role. Everything else stays identical: defensive copy of args, LastIndex-of-ORDER-BY-or-LIMIT injection, UnknownRole → `ErrForbidden`. Cost: ~40 lines plus tests. Pros: minimal blast radius, no API change to the shared helper, easy to delete later if/when `applyAuthFilter` becomes `applyAuthFilter(caller, column, query, args)`. Cons: a second small implementation of the same logic.

- Pros: isolates the "column is `id`, not `client_id`" decision to the file that knows the schema; doesn't perturb booking tests.
- Cons: two helpers with the same shape in the same package; future reader has to know both.
- Effort: **Low**

### 2. Generalize `applyAuthFilter` to take a column name

Refactor signature to `applyAuthFilter(caller *auth.Caller, clientColumn, professionalColumn string, baseQuery string, baseArgs []any) (...)`. Update every caller in `bookings.go` (8 spots). Pros: one helper, easier to grep for. Cons: 8-line test churn in `bookings_test.go`, larger PR (just under 400 LOC review budget) and breaks the rule of "don't refactor unrelated code in the same change."

- Pros: single source of truth for the SQL injection logic; future repos (e.g. a future `notes` table) reuse it.
- Cons: touches `bookings.go` and `bookings_test.go` for a change that doesn't otherwise need them — invites regression risk; PR size creeps toward 400-line review budget.
- Effort: **Medium**

### 3. Skip — declare the debt permanent

Leave `clients.go` and `business_hours_exception.go` without `auth.Caller` wiring. Pros: zero risk, no test churn. Cons: PRD §3.8.7 keeps growing the TODO; Fase 3 admin tools (which will route through these repos for CRUD) inherit a regression risk; the database becomes reachable through MCP code paths with no role enforcement at the persistence edge.

- Pros: nothing changes.
- Cons: perpetuates a documented secu hole for at least one high-impact operation (`BusinessHoursExceptionRepo.Create`/`Delete` DoS via holiday planting); out of step with the rest of the project.
- Effort: **None** (but not viable).

## Recommendation

Approach **1** (mirrored local helper). Scope the change to the two affected repos plus their tests. Refactor `applyAuthFilter` only if a second repo genuinely needs a non-default column — not in this change.

Per-method authorization matrix:

| Method | owner | admin | staff | client |
|---|---|---|---|---|
| `ClientsRepo.Save` | ✅ unrestricted | ✅ unrestricted | ❌ `ErrForbidden` | ❌ `ErrForbidden` (writes are admin-only — mirrors `services.go` Save/Update/Delete) |
| `ClientsRepo.Create` | ✅ | ✅ | ❌ | ❌ |
| `ClientsRepo.FindByID` | ✅ all rows | ✅ all rows | ❌ (no staff↔client link in schema) | `AND id = caller.ClientID` (defense-in-depth; use case already restricts access before this is called) |
| `ClientsRepo.FindByPhone` | ✅ all rows | ✅ all rows | ❌ | ❌ (existence oracle + PII risk; only admin/owner can phone-lookup) |
| `ClientsRepo.GetOrCreate` | ✅ | ✅ | ❌ | ✅ if `caller.ID == phone`, else `ErrForbidden` (a client can only upsert their own phone row) |
| `ClientsRepo.Update` | ✅ | ✅ | ❌ | ❌ (admins own client records; clients edit PII via TUI/delegate pattern, not direct) |
| `ClientsRepo.Delete` | ✅ | ✅ | ❌ | ❌ |
| `ClientsRepo.SearchFTS` | ✅ all rows | ✅ all rows | ❌ | `AND id = caller.ClientID` (only your own FTS hits; also defense against PII leak of `preferences`) |
| `BusinessHoursExceptionRepo.Create` | ✅ | ✅ | ❌ | ❌ (`ErrForbidden` — DoS prevention; holiday planting) |
| `BusinessHoursExceptionRepo.Delete` | ✅ | ✅ | ❌ | ❌ |
| `BusinessHoursExceptionRepo.Get` | ✅ | ✅ | ✅ (read calendar metadata) | ✅ (read calendar metadata) |
| `BusinessHoursExceptionRepo.List` | ✅ | ✅ | ✅ | ✅ |

Rationale for `FindByID` self-scope for clients: matches the bookings precedent (`applyAuthFilter` collapses cross-tenant → `ErrNotFound`, no existence oracle). The `use case` layer that produces the caller context for `ClientsRepo.FindByID` (future Fase 3 admin TUI) will itself restrict access; the repo filter is defense in depth, not the primary gate.

Rationale for `client`-role denial on `Save`/`Create`/`Update`/`Delete` for clients: mirrors `services.go:32/95/123` semantics (writes gated to admin/owner). This project does not currently implement client self-service PII editing; introducing it would be a separate UX/PRD decision.

## Risks

- **Risk:** A `client` caller who hits `ClientsRepo.Update(ctx, c)` with `c.ID == caller.ClientID` might still bypass a row-scope check if `Update`'s SQL is not also rewritten with `AND id = ?`. Mitigation: gate with `RequireRole` only (no scoped column needed) — keeps the auth surface identical to `services.go` writes.
- **Risk:** `business_hours_exception` read paths are needed by `check_availability` on every request; if the wiring accidentally requires a non-`admin`/`-`owner` role there, every LLM request fails. Mitigation: `Get` and `List` must stay open to all authenticated callers — wire with `RequireCaller` (presence check) only, no `RequireRole`.
- **Risk:** FTS returns `preferences` (PII including allergies/medical notes). Admin/owner are full access; clients must NOT see other clients' preferences. Mitigation: `SearchFTS` for clients must add `AND id = caller.ClientID` — a SELECT that filters by `id` AND matches FTS5 returns 0 rows for queries that hit other rows.
- **Risk:** `applyAuthFilter` was designed for tables with a `client_id` column; using it on `clients` (where the row's `id` is the client id) would generate `AND client_id = ?` against a non-existent column and silently return zero rows for the only role that could legitimately hit the API. Mitigation: do NOT reuse `applyAuthFilter` in `clients.go`; replicate the SQL-injection logic with `AND id = ?` instead.
- **Risk:** PR-review budget. This change touches 2 implementation files (~370 LOC total) + 2 test files (~990 LOC). Combined with new auth tests (~+200 LOC), the diff is in the 200–300 LOC range — well below the 400-line review budget, so single-PR delivery is safe under `delivery_strategy=ask-on-risk` and `single-pr` defaults. No need for chained PRs.

## Ready for Proposal

**Yes** — Approach 1 is well-defined, the per-method authorization matrix is pinned to existing patterns, and the diff is well within the 400-line review budget. The next phase (`sdd-propose`) should:

1. Drop a `proposal.md` confirming the auth matrix above and stating that `applyAuthFilter` will NOT be changed.
2. The subsequent `sdd-spec` should add a new spec delta titled `repository-auth-integration` (or extend `auth-roles`/`auth-middleware`) with at minimum these requirements:
   - `ClientsRepo` writes gated to admin/owner.
   - `ClientsRepo.FindByID`/`SearchFTS` apply client-scope `AND id = ?` for the client role.
   - `ClientsRepo.FindByPhone` admin/owner only (PII oracle).
   - `BusinessHoursExceptionRepo.Create`/`Delete` admin/owner only (DoS prevention).
   - `BusinessHoursExceptionRepo.Get`/`List` open to all authenticated callers.
3. `sdd-design`: short. Document the per-method matrix, the `applyAuthFilter`-mirror helper, the test-helper swap from `context.Background()` → `adminCtx()`/`clientCtx(...)`.
4. `sdd-tasks`: small — group by file (clients / business_hours_exception / tests for each). Estimated 10–14 small tasks; no chaining required.
5. `sdd-verify`: assert method behavior under each role using `go-sqlmock` (the existing harness) plus a final `go test -race ./internal/repository/...` and `golangci-lint run`.

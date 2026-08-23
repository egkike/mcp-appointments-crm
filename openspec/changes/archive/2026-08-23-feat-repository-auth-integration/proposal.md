# Proposal: Repository Auth Integration (clients + business_hours_exception)

## Intent

`clients.go` (8 methods) and `business_hours_exception.go` (4 methods) run SQL without consulting `auth.Caller` — the last auth debt (PRD §3.8.7). Any reachable caller can leak PII (`SearchFTS`/`FindByPhone` expose every client's `preferences` — medical notes — and a phone-enumeration oracle) or DoS bookings by planting a closing exception (`9999-12-31`, `is_closed=1`). This breaks repo-layer enforcement of the 3-layer model (PRD §3.8.4) and blocks Fase 3. Fase 2 is unaffected (use cases own filtering).

## Scope

### In Scope
- Auth wiring for all 12 methods per the authorization matrix in `explore.md` §Recommendation.
- Local helper in `clients.go` emitting `AND id = ?` for client self-scope.
- Role-aware tests (admin/owner/staff/client/unauthenticated) for both repos.

### Out of Scope
- New MCP tools, TUI, schema changes, use-case changes, `auth_filter.go` changes, client self-service PII editing.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `clients`: writes + `FindByPhone` admin/owner-only; `FindByID`/`SearchFTS` client self-scope; `GetOrCreate` client-own-phone-only.
- `business-hours-exception`: `Create`/`Delete` admin/owner-only (DoS prevention); `Get`/`List` open to all authenticated callers.

## Approach

**Approach 1 — per-repo local helper (recommended; explore.md §Approaches).**

- `clients.go`: local helper mirroring `applyAuthFilter` (defensive args copy, insert before last `ORDER BY`/`LIMIT`, unknown role → `ErrForbidden`) but emitting `AND id = ?` — `clients` has no `client_id` column; its PK `id` IS the client id, so the shared helper's literal `AND client_id = ?` hits a non-existent column and silently returns zero rows.
- Writes + `FindByPhone`: `auth.RequireRole(ctx, RoleAdmin, RoleOwner)` — Pattern A (`services.go:32`).
- `FindByID`/`SearchFTS`: `RequireCaller` + self-scope — Pattern B (`bookings.go:129–137`); cross-tenant and missing rows collapse to `ErrNotFound` (no oracle).
- BHE `Create`/`Delete`: `RequireRole(admin, owner)`; `Get`/`List`: `RequireCaller` only — the `check_availability` hot path (PRD §3.7.13) must work for every role.

### Alternatives Considered
- **A2 — generalize `applyAuthFilter` with column params:** single source of truth, but touches 12 `bookings.go` call-sites (grep-verified) plus `bookings_test.go` churn for one outlier table — regression risk, budget creep. Rejected: refactor only when a second repo needs a non-default column.
- **A3 — enforce at use-case layer:** rejected — Fase 3 admin tools will call repos directly; PRD §3.8.4 mandates repo-layer enforcement.

## Affected Areas

| Area | Impact |
|------|--------|
| `internal/repository/clients.go` | Modified — auth on 8 methods + scope helper |
| `internal/repository/business_hours_exception.go` | Modified — `RequireRole`/`RequireCaller` on 4 methods |
| `clients_test.go`, `business_hours_exception_test.go` | Modified — role ctx + forbidden/no-caller cases |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Oracle: cross-tenant read returns `ErrForbidden`, not `ErrNotFound` | Medium | Only role gates return `ErrForbidden`; scoped reads collapse to `ErrNotFound` (bookings precedent) |
| FTS filter breaks `MATCH`/`bm25` ordering | Low | Inject `AND id = ?` before `ORDER BY bm25(f)` (`auth_filter.go:79–96` logic); table-driven tests |
| BHE gate breaks `check_availability` hot path | Medium | `Get`/`List` = `RequireCaller` only; existing availability tests guard |
| SQLite busy_timeout under `-race` | Low | No new write paths; WAL/busy_timeout=5000 pragmas unchanged |

## Rollback Plan

`git revert` of the merge commit — no schema change, no dependencies. Both repos return to pre-wiring behavior; Fase 2 tools unaffected either way.

## Dependencies

None new — existing `internal/auth` (`RequireCaller`, `RequireRole`) and role-ctx test helpers (`testutil_test.go:42–67`).

## Success Criteria

- [ ] All 12 methods enforce the `explore.md` authorization matrix.
- [ ] Tests cover admin/owner/staff/client/unauthenticated contexts per gated method.
- [ ] `go test -v -race ./...` and `golangci-lint run ./...` pass.
- [ ] No `go.mod` changes; `auth_filter.go` untouched; no existence oracle on cross-tenant reads.

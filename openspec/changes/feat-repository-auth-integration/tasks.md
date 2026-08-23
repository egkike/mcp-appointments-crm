# Tasks: Repository Auth Integration (clients + business_hours_exception)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~390 (≈90 impl + ≈300 tests) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Test command | Harness | Rollback |
|------|------|--------------|---------|----------|
| 1 | Auth wiring + role matrix tests | `go test -race ./internal/repository/...` | sqlmock; role helpers | `git revert` |

## Phase 1: Foundation — `applyClientsAuthFilter` helper

- [ ] **T-01** Add `applyClientsAuthFilter` in `clients.go` (~55 LOC). Defensive args copy; inject ` AND id = ?` before last `ORDER BY`/`LIMIT` for `RoleClient` (nil `ClientID` → `ErrForbidden`); admin/owner passthrough; staff/unknown → `*domain.SemanticError{ErrCodeForbidden}`.
  - **Dep**: none · **Est**: S · **Accept**: client→clause injected; staff→`ErrForbidden`; nil caller→`ErrUnauthenticated`. **Test**: table-driven (admin/owner/client/staff/nil-ClientID/nil-caller).

## Phase 2: Clients wiring

- [ ] **T-02** `RequireRole(admin, owner)` gate first in `Save`, `Create`, `Update`, `Delete`, `FindByPhone`. SQL unchanged.
  - **Dep**: none · **Est**: S · **Accept**: admin proceeds; others rejected, DB untouched. **Test**: role-matrix per method.

- [ ] **T-03** `FindByID`: `RequireCaller` + `applyClientsAuthFilter`. Cross-tenant→`ErrNotFound`.
  - **Dep**: T-01 · **Est**: S · **Accept**: admin→any row; client→own row; cross-tenant→`ErrNotFound`; staff→`ErrForbidden`. **Test**: regexp `AND id = \?`.

- [ ] **T-04** `SearchFTS`: `RequireCaller` + `applyClientsAuthFilter`. Clause before `ORDER BY bm25(f)`. Client no-match→empty slice, nil error.
  - **Dep**: T-01 · **Est**: S · **Accept**: admin→full ranked; client→own row only; no-match→empty; staff→`ErrForbidden`. **Test**: FTS mock asserting clause placement.

- [ ] **T-05** `GetOrCreate`: inline role switch — admin/owner unrestricted; client iff `phone == *caller.ClientID`; staff→`ErrForbidden`. Gate before validation.
  - **Dep**: none · **Est**: S · **Accept**: client+own-phone→create+idempotent; foreign-phone→`ErrForbidden`; staff→`ErrForbidden`. **Test**: 5-case table per REQ-CL-AUTH-005.

## Phase 3: BHE wiring

- [ ] **T-06** `RequireRole(admin, owner)` first in `Create`/`Delete`. SQL unchanged.
  - **Dep**: none · **Est**: S · **Accept**: admin persists/deletes; others rejected, DB untouched. **Test**: role-matrix + DoS-plant per REQ-BHE-AUTH-001.

- [ ] **T-07** `RequireCaller` (presence only) first in `Get`/`List`. No role check, no filter (hot-path guard).
  - **Dep**: none · **Est**: S · **Accept**: all 4 roles read; caller-less→`ErrUnauthenticated`. **Test**: 5-case table per REQ-BHE-AUTH-002.

## Phase 4: Test migration

- [ ] **T-08** Migrate `clients_test.go` from `context.Background()` to role ctx helpers. Add `ErrUnauthenticated` cases. `ExpectationsWereMet` proves DB untouched.
  - **Dep**: T-02…T-05 · **Est**: M · **Accept**: `go test -race` green; no `context.Background()` on gated methods.

- [ ] **T-09** Migrate `business_hours_exception_test.go` similarly. Add rejection cases for `Create`/`Delete`; all-role reads for `Get`/`List`.
  - **Dep**: T-06, T-07 · **Est**: M · **Accept**: hot-path scenario passes for all four roles.

## Phase 5: Verification

- [ ] **T-10** Run `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`. Confirm zero diff on `auth_filter.go`, `go.mod`, schema.
  - **Dep**: T-01…T-09 · **Est**: S · **Accept**: all five commands exit 0.

## Commit guidance

Single commit `feat(repository): wire auth.Caller into clients and business_hours_exception repos` on `feat/feat-repository-auth-integration-apply`. Single PR ≤ 600 lines.

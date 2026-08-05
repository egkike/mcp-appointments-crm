# Tasks: refactor/clean-architecture

> **Reference**: `proposal.md`, `specs/architecture/spec.md`, `design.md` (D1–D10)
> **Change**: refactor/clean-architecture
> **Status**: Tasks updated — remaining work only
> **Date**: 2026-08-04

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~750–950 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (P3.3) → PR 2 (P3.4a-d) → PR 3 (P4.1+P4.2) — final |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | P3.3 domain entity enrichment | PR 1 | `go test -v -race ./internal/domain/entity/...` | N/A (pure unit tests) | Revert `internal/domain/entity/*` changes + repo validator callers |
| 2 | P3.4 infra cleanup (uuid/errors/validation) | PR 2 | `go test -v -race ./internal/repository/...` | N/A (sqlmock tests) | Revert `internal/idgen/uuid.go`, `internal/repository/sqlite_errors.go`, caller updates |
| 3 | P3.4c delete `internal/model/` + verify | PR 2 (folded) | `go test -v -race ./...` | `go build ./...` | Revert `internal/repository/*.go` `model→entity` changes; restore `internal/model/` |
| 4 | P4 composition root | PR 3 (final) | `go test -v -race ./...` | `go run ./cmd/mcp-server` exits 0 | Delete `cmd/mcp-server/main.go` |

## Completed

- [x] P1 — Domain layer (entities, repository interfaces, errors, AvailabilityService)
- [x] P2 — Application layer (DTOs + use cases)
- [x] P3.1 — Auth helpers moved to `internal/auth/`
- [x] P3.2 — Domain interfaces implemented in all repos
- [x] P3.3a — Removed zombie `BookingsRepo.CheckAvailability`

## Phase 3 — Repository Refactor (remaining)

### P3.3 — Move business logic out of repos

- [x] P3.3b — Add `Booking.CanTransitionTo(status)` and `Booking.ValidDuration()` to `internal/domain/entity/booking.go`; write RED→GREEN tests.
- [x] P3.3c — Move cross-entity validators to domain methods and update repo callers (RED→GREEN):
  - [x] `Service.Validate()` replacing `validateService` in `services.go`
  - [x] `Professional.Validate()` replacing `validateProfessional` in `professionals.go`
  - [x] `Account.Validate()` replacing `validateAccount` in `accounts.go`
  - [x] `Booking.IsValidTimeRange()` replacing `validateScheduleTimes` in `schedules.go`
  - [x] `BusinessProfile.Validate()` replacing `validateBusinessProfile` in `business_profile.go`
  - [x] `PendingAlert.IsValidType()` replacing `validateAlertType` in `pending_alerts.go`
- [x] P3.3d — Document: keep `ClientsRepo.GetOrCreate` as storage pattern (not business logic).
  > **Decision**: `ClientsRepo.GetOrCreate` is an idempotent storage pattern (INSERT OR RETURN EXISTING) that belongs in the repository layer. It is not business logic — it is a data access optimization that avoids a separate SELECT-before-INSERT round-trip. The domain entities do not need a `GetOrCreate` method because the semantics are purely about storage, not about domain invariants.

### P3.4 — Delete obsolete files

- [x] P3.4a — Migrate `model.NewUUID()` callers at `internal/repository/clients.go:117` and `internal/repository/professionals.go:85` to `idgen.NewUUID()` (add wrapper in `internal/idgen/uuid.go` if missing).
  > **Actual impl**: Added `idgen.NewUUID()` wrapper (returns string, discards error, calls `New()`). Migrated `clients.go:117` and `professionals.go:71` (corrected line number — was 85 in tasks, actual is 71). Added `TestNewUUID` (3 subtests: non-empty UUID v4, format matches New(), uniqueness across 10k calls). Removed `model` import from `professionals.go` (sole usage migrated).

- [x] P3.4b — Delete `internal/repository/errors.go`; move `isUniqueViolation()` + `sqliteConstraintUnique` to new `internal/repository/sqlite_errors.go`; update `isSingleOwnerViolation()` in `accounts.go` to live alongside SQLite helpers; migrate `errors_test.go`.
  > **Actual impl**: Created `doc.go` (package comment), `sqlite_errors.go` (isUniqueViolation, sqliteConstraintUnique, isSingleOwnerViolation moved from accounts.go). Deleted `errors.go`. `git mv errors_test.go → sqlite_errors_test.go`. Removed unused `strings` import from `accounts.go`.

- [x] P3.4c — Delete `internal/model/` (10 files, 203 LOC): update `clients.go`, `professionals.go`, `services.go`, and `clients_test.go` to use `entity.*` equivalents; remove `github.com/google/uuid` from `go.mod` if unused.
  > **Actual impl**: Migrated ALL `model.*` references in `clients.go` (Create/GetOrCreate/Update/SearchFTS → entity.Client), `services.go` (SearchFTS → entity.Service, s.IsActive → s.Active), and `clients_test.go` (10 × model.Client → entity.Client). Set `Active: true` on entity.Client construction (clients table has no is_active column). Removed 11 files from `internal/model/`. Ran `go mod tidy` — `google/uuid` stays as indirect dep of `modernc.org/sqlite`.

- [x] P3.4d — Decide `datePattern`/`timeHHMMRe` destination: keep package-private in `internal/repository/validation.go` OR move to `internal/validation/`; document decision in code comment and update tasks.
  > **Decision**: KEPT in `internal/repository/validation.go`. These regexes are only used by the repository layer to validate data arriving from SQLite (date strings, FTS5 query syntax). Documented rationale in code comment at top of `validation.go`.

### P3.5 — Verify Phase 3

- [x] P3.5a — `go build ./...` passes
- [x] P3.5b — `go test -v -race ./...` passes (9 packages, 0 failures, 0 races)
- [x] P3.5c — `grep -r 'database/sql' internal/domain/` returns empty (only a doc comment mention, no real imports)
- [x] P3.5d — All `var _ domain.XxxRepository = (*Impl)(nil)` compile checks pass

## Phase 4 — DI Wiring

- [x] P4.1 — Create `cmd/mcp-server/main.go` as composition root only: open SQLite via `internal/db`, construct all repos, construct all use cases, verify no concrete type leaks beyond `cmd/`, exit 0. No SSE server, no `internal/mcp/`.
  > **Actual impl**: Created `cmd/mcp-server/main.go` (142 lines). Wires 9 repos (`NewAccountsRepo` with logger, 8 others plain). Constructs `BookingValidator` once (TASK-FU.3 singleton), shared between `CreateBookingUseCase` and `RescheduleBookingUseCase` (both 7 args). Wires `CheckAvailabilityUseCase` with `AvailabilityChecker` interface + `AvailabilityDeps` struct. `GetBooking` and `CancelBooking` receive only `BookingsRepo`. DB path: `MCP_DB_PATH` env var or `./data/appointments.db` default. Logger: `slog.Default()`. Exit 0 on success with INFO line; exit 1 on DB open failure. Decision on `bookingValidator` interface (D4): KEPT as narrow contract in `internal/application/usecase/validator.go` — `domain.BookingValidator` deferred until a third consumer appears (documented in code comment per TASK-FU.3 resolution). No `init()` functions, no DI containers, no reflection. No SSE server or `internal/mcp/`.
  - **Wiring reminder check**: All 7 items from the reminder block satisfied. `accountsRepo`, `clientsRepo`, `pendingAlertsRepo` are constructed but not yet wired to use cases (future MCP transport layer consumers).

- [x] P4.2 — Verify Phase 4: `go build ./...` passes; `go test -v -race ./...` passes (9 packages); no `init()` functions for DI (`grep -r 'func init()' cmd/` empty); `go run ./cmd/mcp-server` exits 0 with `composition root wired successfully repos=9 usecases=5 booking_validator_shared=true`; `go vet ./...` clean; `golangci-lint run ./...` 0 issues.

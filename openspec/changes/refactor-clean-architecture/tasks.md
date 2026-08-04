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
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 |
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
| 3 | P3.4c delete `internal/model/` + verify | PR 3 | `go test -v -race ./...` | `go build ./...` | Revert `internal/repository/*.go` `model→entity` changes; restore `internal/model/` |
| 4 | P4 composition root | PR 4 | `go test -v -race ./...` | `go run ./cmd/mcp-server` exits 0 | Delete `cmd/mcp-server/main.go` |

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

- [ ] P3.4a — Migrate `model.NewUUID()` callers at `internal/repository/clients.go:117` and `internal/repository/professionals.go:85` to `idgen.NewUUID()` (add wrapper in `internal/idgen/uuid.go` if missing).
- [ ] P3.4b — Delete `internal/repository/errors.go`; move `isUniqueViolation()` + `sqliteConstraintUnique` to new `internal/repository/sqlite_errors.go`; update `isSingleOwnerViolation()` in `accounts.go` to live alongside SQLite helpers; migrate `errors_test.go`.
- [ ] P3.4c — Delete `internal/model/` (10 files, 203 LOC): update `clients.go`, `professionals.go`, `services.go`, and `clients_test.go` to use `entity.*` equivalents; remove `github.com/google/uuid` from `go.mod` if unused.
- [ ] P3.4d — Decide `datePattern`/`timeHHMMRe` destination: keep package-private in `internal/repository/validation.go` OR move to `internal/validation/`; document decision in code comment and update tasks.

### P3.5 — Verify Phase 3

- [ ] P3.5a — `go build ./...` passes
- [ ] P3.5b — `go test -v -race ./...` passes
- [ ] P3.5c — `grep -r 'database/sql' internal/domain/` returns empty
- [ ] P3.5d — All `var _ domain.XxxRepository = (*Impl)(nil)` compile checks pass

## Phase 4 — DI Wiring

- [ ] P4.1 — Create `cmd/mcp-server/main.go` as composition root only: open SQLite via `internal/db`, construct all repos, construct all use cases, verify no concrete type leaks beyond `cmd/`, exit 0. No SSE server, no `internal/mcp/`.
- [ ] P4.2 — Verify Phase 4: `go build ./...` passes; `go test -v -race ./...` passes; no `init()` functions for DI.

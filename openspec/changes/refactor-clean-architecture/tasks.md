# Tasks: refactor/clean-architecture

> **Reference**: `proposal.md`, `specs/architecture/spec.md`, `design.md`
> **Change**: refactor/clean-architecture
> **Status**: Tasks defined
> **Date**: 2026-07-29

## Phase 1 — Domain Layer (additive)

### P1.1 — Create domain entity files

For each model, create `internal/domain/entity/{name}.go`:
- Copy the struct from `internal/model/`
- Add at least one business method (e.g., `CanTransitionTo(status)`, `IsValid()`, `ServiceDuration()`)
- Remove `json:` and `db:` tags
- Keep the model file in place for backward compatibility

| File | Entity | Suggested methods |
|------|--------|-------------------|
| `domain/entity/booking.go` | Booking | `CanCancel()`, `CanReschedule()`, `IsOverlapping(other)` |
| `domain/entity/client.go` | Client | `IsActive()`, `HasValidPhone()` (phone format validation on entity field) |
| `domain/entity/professional.go` | Professional | `IsActive()`, `HasSpecialty(serviceID)` |
| `domain/entity/service.go` | Service | `IsActive()`, `Duration() time.Duration` |
| `domain/entity/business_profile.go` | BusinessProfile | `IsOpenOn(dayOfWeek)`, `GetOpenClose(dayOfWeek)` |
| `domain/entity/business_hours_exception.go` | BusinessHoursException | `IsClosed()`, `EffectiveHours()` |
| `domain/entity/pending_alert.go` | PendingAlert | `IsDue()`, `CanBeSent()` |
| `domain/entity/schedule.go` | Schedule | `IncludesTime(hhmm string)` |
| `domain/entity/account.go` | Account | `IsActive()`, `HasRole(role)` |

- [x] P1.1a — Create `domain/entity/booking.go` with CanCancel, CanReschedule, IsOverlapping
- [x] P1.1b — Create `domain/entity/client.go` with IsActive, HasValidPhone
- [x] P1.1c — Create `domain/entity/professional.go` with IsActive, HasSpecialty
- [x] P1.1d — Create `domain/entity/service.go` with IsActive, Duration
- [x] P1.1e — Create `domain/entity/business_profile.go` with IsOpenOn, GetOpenClose
- [x] P1.1f — Create `domain/entity/business_hours_exception.go` with IsClosed, EffectiveHours
- [x] P1.1g — Create `domain/entity/pending_alert.go` with IsDue, CanBeSent
- [x] P1.1h — Create `domain/entity/schedule.go` with IncludesTime
- [x] P1.1i — Create `domain/entity/account.go` with IsActive, HasRole

### P1.2 — Create domain errors

- [x] P1.2a — Create `internal/domain/errors.go` with SemanticError struct + sentinel errors + error codes
- [x] P1.2b — Ensure all error codes from `internal/repository/errors.go` are duplicated (don't delete originals yet)
- [x] P1.2c — Consolidate the two `ErrUnauthenticated` definitions: pick one canonical message ("caller not authenticated") and use it in `domain/errors.go`; the repo version (`auth_helpers.go:14`) and auth version (`resolver.go:12`) will be deleted/updated in Phase 3
- [x] P1.2d — Note: `ErrCode` from repo/errors.go and `ErrCode` from domain/errors.go are distinct types. P3.4a must update all consumers to use the domain type, with explicit string conversion at the repo boundary (`domain.ErrCode(originalCode)`)

### P1.3 — Create domain repository interfaces

For each domain aggregate, create `internal/domain/repository/{name}.go`:

- [x] P1.3a — `domain/repository/bookings.go` (FindByID, Create, Update, Cancel, Reschedule, FindOverlapping, FindByStaffAndRange, ListBookingsForRange, SearchByNotes, UpdateStatus)
  > Must cover all methods that use cases and domain service need (create, cancel, reschedule, check_availability, get_booking, list, search). Mirror the current BookingsRepo method set with domain-centric naming. `Save` is split into `Create` (insert-only) and `Update` (status changes) to match the SQL boundary.
  > Actual impl: 10 methods, all use `context.Context` as first arg; `id` is `string` (matches `Booking.ID string`). 46 LOC.
- [x] P1.3b — `domain/repository/clients.go` (FindByID, FindByPhone, Save)
  > Actual impl: 3 methods, `id` is `string` (matches `Client.ID string`). 20 LOC.
- [x] P1.3c — `domain/repository/professionals.go` (FindByID, FindActive, Save, Update)
  > Actual impl: 4 methods, `id` is `string` (matches `Professional.ID string`). 23 LOC.
- [x] P1.3d — `domain/repository/services.go` (FindByID, FindActive, Save, Update, Delete)
  > Actual impl: 5 methods, `id` is `string` (matches `Service.ID string`). 26 LOC.
- [x] P1.3e — `domain/repository/business_profile.go` (Get, Update)
  > Actual impl: 2 methods, no ID arg (singleton aggregate). 18 LOC.
- [x] P1.3f — `domain/repository/business_hours_exception.go` (Get, Create, List, Delete)
  > Current codebase has no `FindByDate` method. If needed by a use case, add in Phase 2 with new SQL.
  > Actual impl: 4 methods. `Get`/`Delete` use `id int` (matches `BusinessHoursException.ID int`); `List` uses `from, to time.Time` range. 25 LOC.
- [x] P1.3g — `domain/repository/pending_alerts.go` (Save, FindPending, MarkAsSent, Cancel)
  > Actual impl: 4 methods. `MarkAsSent`/`Cancel` use `id int` (matches `PendingAlert.ID int`); `FindPending` uses `now time.Time` cutoff. 24 LOC.
- [x] P1.3h — `domain/repository/schedules.go` (FindByProfessionalAndDay, Upsert, Delete)
  > Actual impl: 3 methods. `FindByProfessionalAndDay` and `Delete` use `day int` (matches `Schedule.DayOfWeek int` 0-6, not `time.Weekday`). 20 LOC.
- [x] P1.3i — `domain/repository/accounts.go` (FindByID, Create, GetByRole, List, Update, Deactivate, IsActive, ListByProfessional)
  > Note: no `FindByEmail` exists in the current codebase. If required, add in Phase 5 with new SQL/endpoint. The interface must cover ALL methods the use cases need: `Create`, `Get`, `Update`, `Deactivate`, `IsActive`, `GetByRole`, `List`, `ListByProfessional`.
  > Actual impl: 8 methods. `GetByRole(ctx, role string)` uses plain `string` (no `AccountRole` type exists in entity — `Account.Role string` with comment `"owner" | "admin" | "staff"`). **Follow-up in P1.5**: introduce `type AccountRole string` in `entity/account.go` for type-safety and update `GetByRole` to `entity.AccountRole`. `ListByProfessional(professionalID string)`. 37 LOC.

### P1.4 — Create domain service: AvailabilityService

- [x] P1.4a — Create `internal/domain/service/availability.go`
  > Actual impl: 219 LOC, `AvailabilityService` struct (stateless) + `CheckAvailability(ctx, params, deps) (*Result, error)` method, plus `CheckAvailabilityParams`/`CheckAvailabilityResult`/`AvailabilityDeps` types. Pure Go, zero infra imports.
- [x] P1.4b — Move CheckAvailability business rules from `BookingsRepo.CheckAvailability` into the domain service
  > Actual impl: 5-step chain ported byte-for-byte (input resolution, 3a business hours, 3b pro schedule, 3c slot within hours, 3d overlap, 3e past). Same error codes and same Spanish messages as `repository/bookings.go:CheckAvailability` (one minor deviation: overlap message uses `time.RFC3339` instead of infra `FormatStorage`).
- [x] P1.4c — Domain service receives repo interfaces as method arguments (not constructor)
  > Actual impl: chose `AvailabilityDeps` struct over 6 individual method args (readability, extensibility, easier mocking). Deps has 6 fields: `Services`, `Professionals`, `BusinessProfile`, `BusinessHoursExceptions`, `Schedules`, `Bookings`. Constructor `NewAvailabilityService()` is a no-arg factory.
- [x] P1.4d — Write pure unit tests for availability service with mock repo interface
  > Actual impl: 12 `CheckAvailability` table-driven scenarios (happy path + 11 error cases) + 4 `hhmmToMinutes` cases. 16/16 subtests pass. Mocks are hand-rolled (no third-party mocking lib) using function-table style in `mocks_test.go` (100 LOC).

### P1.5 — Verify Phase 1

- [x] P1.5a — `go build ./...` passes (old code still works)
  > Verified: build clean after AccountRole typing.
- [x] P1.5b — `go test -race ./...` passes
  > Verified: all packages pass with -race. 7 packages OK, 0 failures.
- [x] P1.5c — `grep -r 'database/sql' internal/domain/` returns empty
  > Verified: only an explanatory comment in entity/errors.go about zero-dependency rule. No actual imports.
- [x] P1.5d — Save progress to engram
  > Done: mem_save patterns for JD round 1+2, GGA 5-warning amend flow, sdd-attempt ledger quirks; mem_session_summary at end of session.
- [x] P1.5e — Type `AccountRole` as `entity.AccountRole` enum (follow-up from P1.3i)
  > Actual impl: added `type AccountRole string` with `RoleOwner`/`RoleAdmin`/`RoleStaff` constants in `entity/account.go`. Changed `Account.Role` and `HasRole(role)` to use `AccountRole`. Updated `AccountsRepo.GetByRole` signature. Updated `account_test.go` table to use constants.

> **Datetime type handling**: As each entity file is created, convert string datetime fields to `time.Time`. The `string ↔ time.Time` conversion happens at the repo SQL-scan boundary (unchanged in Phase 1 since repos still work with model structs). Exceptions: date-only and time-of-day fields (`BusinessHoursException.Date`, `BusinessProfile.OpenTime/CloseTime`, `Schedule.StartTime/EndTime`) keep `string` with format validation.

---

## Phase 2 — Application Layer (additive)

### P2.1 — Create DTOs

- [x] P2.1a — Create `internal/application/dto/create_booking.go` (CreateBookingInput, CreateBookingResult)
- [x] P2.1b — Create `internal/application/dto/cancel_booking.go`
- [x] P2.1c — Create `internal/application/dto/reschedule_booking.go`
- [x] P2.1d — Create `internal/application/dto/check_availability.go` (CheckAvailabilityParams, CheckAvailabilityResult)
- [x] P2.1e — Create `internal/application/dto/get_booking.go`

### P2.2 — Create use cases

- [x] P2.2a — Create `internal/application/usecase/create_booking.go` (constructor + Execute)
- [x] P2.2b — Create `internal/application/usecase/cancel_booking.go`
- [x] P2.2c — Create `internal/application/usecase/reschedule_booking.go`
- [x] P2.2d — Create `internal/application/usecase/check_availability.go`
- [x] P2.2e — Create `internal/application/usecase/get_booking.go`

### P2.3 — Verify Phase 2

- [x] P2.3a — `go build ./...` passes
- [x] P2.3b — `go test -race ./...` passes
- [x] P2.3c — No infra imports in `internal/application/`
- [x] P2.3d — Write use case unit tests with mock repos

---

## Phase 3 — Refactor Repositories (modifies existing code)

### P3.1 — Move auth helpers to auth

- [x] P3.1a — Export `requireCaller`, `requireRole`, and `requireClientMatch` from `internal/auth/`
- [x] P3.1b — Update `internal/repository/*.go` to import auth helpers from `internal/auth/` instead of local package
- [x] P3.1c — Consolidate `ErrUnauthenticated`: use the canonical one from `internal/domain/errors.go`, delete the definitions in `auth_helpers.go` and `auth/resolver.go`
- [x] P3.1d — Move `internal/repository/auth_helpers_test.go` (260 lines) to `internal/auth/auth_helpers_test.go` and update imports
- [x] P3.1e — Delete `internal/repository/auth_helpers.go`

### P3.2 — Implement domain interfaces in repos

Each repo must add interface conformance and rename methods to match domain interface. For each repo with renamed methods, update the corresponding `*_test.go` file to call the new names:

- [x] P3.2a — `BookingsRepo`: implement `domain.BookingRepository`, rename methods (GetBooking→FindByID, CreateBooking→Create, CancelBooking→Cancel, RescheduleBooking→Reschedule, etc.), update `bookings_test.go`
  > Actual impl: 10 methods implemented (4 with signature changes to entity.Booking/time.Time, 6 new from scratch: Update, FindOverlapping, FindByStaffAndRange, ListBookingsForRange, SearchByNotes, UpdateStatus). Compile-time assertion `var _ domainrepo.BookingsRepo = (*BookingsRepo)(nil)` added. time.Time conversion at SQL boundary via FormatStorage/parseStorageTime helpers and scanBooking utility. bookings_test.go rewritten for new signatures plus 6 new method tests. CheckAvailability preserved per P3.3a. Pre-flight clean: go fmt/vet/build/lint/test -race all pass.
- [x] P3.2b — `ClientsRepo`: implement `domain.ClientRepository`, rename methods (Get→FindByID, GetByPhone→FindByPhone, added Save as upsert), update `clients_test.go`
  > Actual impl: FindByID/FindByPhone return entity.Client (Active defaults true — no SQL column). Save uses INSERT OR REPLACE. Extra methods (Create, GetOrCreate, Update, Delete, SearchFTS) kept with model types — out of scope. Compile-time assertion added.
- [x] P3.2c — `ProfessionalsRepo`: implement `domain.ProfessionalRepository`, update `professionals_test.go`
  > Actual impl: Get→FindByID, GetActive→FindActive, Create→Save. Update kept name, param changed to entity.Professional. model import kept for model.NewUUID(). Compile-time assertion added.
- [x] P3.2d — `ServicesRepo`: implement `domain.ServiceRepository`, update `services_test.go`
  > Actual impl: Get→FindByID, ListActive→FindActive, Create→Save. entity.Active maps to SQL is_active column. SearchFTS kept with model types. Compile-time assertion added.
- [x] P3.2e — `BusinessProfileRepo`: implement `domain.BusinessProfileRepository`, update `business_profile_test.go`
  > Actual impl: GetBusinessProfile→Get, UpdateBusinessProfile→Update. Entity fields match model exactly — straightforward rename. Compile-time assertion added.
- [x] P3.2f — `BusinessHoursExceptionRepo`: implement `domain.BusinessHoursExceptionRepository`, update `business_hours_exception_test.go`
  > Actual impl: GetByDate→Get (param string→time.Time, converts via date.Format("2006-01-02")), List adds from/to time.Time params with WHERE clause, Delete param int64→int. Create param changed to entity. Compile-time assertion added.
- [x] P3.2g — `PendingAlertsRepo`: implement `domain.PendingAlertRepository`, update `pending_alerts_test.go`
  > Actual impl: Create→Save (param model→entity, ScheduledDatetime time.Time→string via FormatStorage). ListPending→FindPending (removed limit param, beforeTime string→now time.Time, scans datetime string→time.Time via parseStorageTime). Compile-time assertion added.
- [x] P3.2h — `SchedulesRepo`: implement `domain.ScheduleRepository`, update `schedules_test.go`
  > Actual impl: GetByProfessionalAndDay→FindByProfessionalAndDay. Upsert param changed to entity.Schedule. Entity fields match model exactly. Compile-time assertion added.
- [ ] P3.2i — `AccountsRepo`: implement `domain.AccountRepository`, update `accounts_test.go`

### P3.3 — Move business logic out of repos

- [ ] P3.3a — Remove `CheckAvailability` from BookingsRepo (delegate to `domain/service/availability.go`)
- [ ] P3.3b — Move entity creation validation (status FSM, duration check, overlap detection) to domain entity methods (`Booking.CanTransitionTo`, `Booking.IsOverlapping`, `Booking.ValidDuration`)
- [ ] P3.3c — Move cross-entity validators to domain service or entity:
  - `validateService(serviceID)` → `entity.Service.IsActive()` + domain service
  - `validateProfessional(professionalID)` → `entity.Professional.IsActive()`
  - `validateAccount(accountID)` → `entity.Account.IsActive()` + role check
  - `validateScheduleTimes(start, end)` → `entity.Booking.IsValidTimeRange()`
  - `validateBusinessProfile(businessProfileID)` → `entity.BusinessProfile.IsOpenOn()`
  - `validateAlertType(alertType)` → `entity.PendingAlert.IsValidType()`
- [ ] P3.3d — Keep `GetOrCreate` on ClientsRepo (it's an infra concern: "find or insert" is a storage pattern, not business logic)

### P3.4 — Delete obsolete files

- [ ] P3.4a — Relocate `internal/model/uuid.go` to `internal/id/uuid.go` (or `internal/uuid/id.go` — a thin helper, not a domain concept). Update all callers (`bookings.go:97`, `clients.go:92`, `professionals.go:79`)
- [ ] P3.4b — Delete `internal/repository/errors.go` after migrating:
  - Errors → `internal/domain/errors.go` (consumers update import path)
  - `isUniqueViolation()` + `sqliteConstraintUnique()` → keep in `internal/repository/sqlite_errors.go` (infra-level helpers, don't belong in domain)
  - Update `isSingleOwnerViolation()` in `accounts.go:340` similarly
  - Update all consumers: change `repository.ErrCode*` references to `domain.ErrCode*` with explicit conversion `domain.ErrCode(code)` at repo boundary
- [ ] P3.4c — Delete `internal/model/`:
  1. First update ALL imports: every `internal/repository/*.go` file that imports `model` → `domain/entity`
  2. Update `*_test.go` imports similarly
  3. Remove `internal/model/` directory
  4. Verify build passes
- [ ] P3.4d — Update `internal/repository/validation.go` to relocate format validators (`datePattern`, `timeHHMMRe`) to `internal/validation/` or keep in repo as package-private utils; document decision

### P3.5 — Verify Phase 3

- [ ] P3.5a — `go build ./...` passes
- [ ] P3.5b — `go test -race ./...` passes
- [ ] P3.5c — `grep -r 'database/sql' internal/domain/` returns empty
- [ ] P3.5d — All `var _ domain.XxxRepository = (*Impl)(nil)` compile checks pass

---

## Phase 4 — Fix DI Wiring

### P4.1 — Wire main.go

- [ ] P4.1a — Create `cmd/mcp-server/main.go` with DI: construct repos → construct use cases → inject into MCP handlers
- [ ] P4.1b — Ensure only `cmd/` knows about concrete types

### P4.2 — Verify Phase 4

- [ ] P4.2a — `go build ./...` passes
- [ ] P4.2b — `go test -race ./...` passes
- [ ] P4.2c — No `init()` functions for DI

---

## Sub-PR Plan (R4 Compliance)

Each phase exceeding the 600-line review budget MUST be split into independent sub-PRs:

| Phase | Total LOC | Sub-PR | Scope | Est. LOC |
|-------|-----------|--------|-------|----------|
| **P1** | ~800 | P1a (PR #1) | Entities (P1.1) + errors (P1.2) | ~400 |
| | | P1b (PR #2) | Repository interfaces (P1.3) + domain service (P1.4) | ~400 |
| **P2** | ~600 | P2a (PR #3) | DTOs (P2.1) | ~200 |
| | | P2b (PR #4) | Use cases (P2.2) + tests (P2.3d) | ~400 |
| **P3** | ~500 | P3a (PR #5) | Auth helpers move (P3.1) + errors/uuid relocation (P3.4) | ~200 |
| | | P3b (PR #6) | Domain interface impl (P3.2) + business logic move (P3.3) + delete model (P3.4c) | ~300 |
| **P4** | ~100 | P4 (PR #7) | DI wiring | ~100 |

**Rules**:
- Each sub-PR MUST pass `go test -race ./...` independently
- Each sub-PR MUST be mergeable to main without breaking existing functionality
- Sub-PRs within a phase are ORDERED — P1a must merge before P1b
- Phase ordering is preserved across the whole change

## Summary

| Phase | Files | Est. LOC | Risk |
|-------|-------|----------|------|
| P1 | ~22 new | ~800 | None (additive) |
| P2 | ~12 new | ~600 | None (additive) |
| P3 | ~15 modified | ~500 changed | Medium |
| P4 | ~2 modified | ~100 | Low |
| **Total** | **~51** | **~2000** | |

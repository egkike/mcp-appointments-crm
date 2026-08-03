# Design: BookingValidator Domain Service

> **Change**: feat-booking-validator-service
> **Status**: Designed
> **Date**: 2026-08-03
> **Related**: Issues #22, #23 · PRs #33, #34, #35, #36 (refactor-clean-architecture)
> **Depends on**: refactor-clean-architecture P3.3a (merged as #36)

## Section 1: Architectural Overview

This change introduces a single source of truth for booking-datetime validation
in the domain layer and routes the two silent booking use cases
(`CreateBooking`, `RescheduleBooking`) through it before repo dispatch.

### Three-layer placement

```
┌─────────────────────────────────────────────────────────────┐
│ cmd/mcp-server/main.go (P4 wiring)                          │
│   NewBookingValidator() injected into use cases             │
├─────────────────────────────────────────────────────────────┤
│ internal/application/usecase/                               │
│   CreateBookingUseCase  ──┐                                 │
│   RescheduleBookingUseCase─┴──▶ BookingValidator.Validate() │
│   CheckAvailabilityUseCase ──▶ AvailabilityService (UNTOUCHED)│
├─────────────────────────────────────────────────────────────┤
│ internal/domain/service/                                    │
│   BookingValidator.Validate()     ──┐                       │
│   AvailabilityService.CheckAvailability──┴──▶ ValidateBookingTimeSlot()
│   (pure helper — package-private, no I/O, no state)         │
├─────────────────────────────────────────────────────────────┤
│ internal/domain/repository/  (interfaces — UNCHANGED)       │
│   BookingsRepo.FindOverlapping()                            │
├─────────────────────────────────────────────────────────────┤
│ internal/repository/bookings.go  (UNCHANGED)                │
│   atomic WHERE NOT EXISTS overlap guard — defense-in-depth  │
└─────────────────────────────────────────────────────────────┘
```

### Data flow after PR #B/#C

```
UseCase.Execute
   │
   ├── auth.RequireAuthenticated / AuthorizeBookingAccess   (unchanged)
   ├── resolve svc, pro, business profile, schedules        (use case)
   ├── uc.validator.Validate(ctx, ValidateBookingInput)
   │        │
   │        └──▶ ValidateBookingTimeSlot(ctx, slot, deps)
   │                      │
   │                      └──▶ Bookings.FindOverlapping()  (mock-friendly)
   │
   ├── on validator error ──▶ return *SemanticError unchanged (no mapping)
   ├── on validator pass  ──▶ uc.bookings.Create / .Reschedule
   │            └── on domain.ErrConflict (TOCTOU) ──▶ map to ErrCodeBookingOverlap
   └── return result
```

### Dependency direction

- Outer layers (`cmd`, `application`) depend on `internal/domain/service` interfaces
  and `internal/domain/repository` interfaces.
- `internal/domain/service/` depends ONLY on `internal/domain/`, `internal/domain/entity/`,
  `internal/domain/repository/`, and the standard library.
- `database/sql` is FORBIDDEN in `internal/domain/service/` (REQ-BV-5; zero-dependency rule).
- The new files add NO import of `internal/repository/` (concrete SQL) — only the abstractions in
  `internal/domain/repository/`.

## Section 2: Module Layout

Exact tree for the 3 PRs (parent dirs existing, unchanged files tagged `UNTOUCHED`):

```
internal/domain/service/
├── availability.go                    (MODIFIED — mechanical refactor in PR #A)
├── availability_test.go               (UNTOUCHED — regression gate, 16 existing t.Run invocations: 15 inside `TestCheckAvailability` + 1 `TestHHMMToMinutes`)
├── booking_validator.go               (NEW — PR #A)
├── booking_validator_test.go          (NEW — PR #A)
├── booking_time_validator.go          (NEW — PR #A)
├── booking_time_validator_test.go     (NEW — PR #A)
├── datetime_helpers.go                (UNTOUCHED — reused as-is)
└── mocks_test.go                      (EXTENDED in PR #B/#C if a use-case-package
                                        mock is added; the domain-service-package
                                        mocks are already complete for PR #A)

internal/application/usecase/
├── create_booking.go                  (MODIFIED — PR #B, +1 field, +1 ctor param)
├── create_booking_test.go             (EXTENDED — PR #B, +8 subtests)
├── reschedule_booking.go              (MODIFIED — PR #C, +1 field, +1 ctor param)
├── reschedule_booking_test.go         (EXTENDED — PR #C, +8 subtests)
└── check_availability.go              (UNTOUCHED)
```

Infrastructure / repo / cmd:

```
internal/repository/bookings.go         (UNTOUCHED — atomic guard stays as defense-in-depth)
internal/domain/repository/bookings.go (UNTOUCHED — BookingsRepo interface unchanged)
internal/domain/errors.go              (UNTOUCHED — 7 ErrCode* constants preserved)
cmd/mcp-server/main.go                 (PR #B adds the validator to the
                                        CreateBookingUseCase ctor; PR #C routes it
                                        into RescheduleBookingUseCase. The shared
                                        BookingValidator single instance is created
                                        once and passed to both use cases.)
```

## Section 3: Interface Contracts

All signatures are Go-flavored. Types reference existing packages:
`github.com/egkike/mcp-appointments-crm/internal/domain`,
`.../internal/domain/entity`, `.../internal/domain/repository`.

### 3.1 `BookingValidator` (struct + method) — PR #A

```go
// internal/domain/service/booking_validator.go
package service

import (
    "context"
    "time"

    "github.com/egkike/mcp-appointments-crm/internal/domain"
    "github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// BookingValidator is the concrete implementation of the domain.BookingValidator
// interface (declared in internal/domain/booking_validator.go, the zero-dep
// package). It orchestrates datetime validation for booking mutations.
// Stateless: holds no *sql.DB, no repo fields. Safe to share as a singleton.
//
// IMPORTANT: use cases depend on the INTERFACE in `internal/domain/`, not on
// this struct. This avoids the import cycle use case → domain/service →
// domain/repository (use case already imports the latter). The interface is
// satisfied structurally by *BookingValidator — no explicit "implements" clause.
type BookingValidator struct{}

// NewBookingValidator returns a stateless BookingValidator.
func NewBookingValidator() *BookingValidator { return &BookingValidator{} }

// ValidateBookingInput carries the proposed slot and all entities the use case
// has already resolved. No DB lookups happen inside Validate.
type ValidateBookingInput struct {
    Service            *entity.Service
    Professional       *entity.Professional
    BusinessProfile    *entity.BusinessProfile
    ProfessionalSchedule *entity.Schedule     // for the slot's weekday
    Exception          *entity.BusinessHoursException // nil if none for the date
    NewStart           time.Time             // already in business timezone semantics
    Bookings           BookingOverlapReader  // narrow read interface (see 3.2)
}

// Validate runs the 5-step chain via the shared helper. Returns the first
// *domain.SemanticError encountered, or nil on success. It does NOT validate
// service/professional active status — that stays in the use case.
func (v *BookingValidator) Validate(
    ctx context.Context,
    input ValidateBookingInput,
) *domain.SemanticError

// Mapping from ValidateBookingInput to SlotInput (consumed by the helper):
//   input.NewStart          -> slot.Start
//   input.Service           -> slot.Service
//   input.Professional      -> slot.Professional
//   input.BusinessProfile   -> slot.BusinessProfile
//   input.ProfessionalSchedule -> slot.Schedule
//   input.Exception         -> slot.Exception
//   input.Bookings          -> wrapped into BookingTimeValidatorDeps.Bookings
// Note: input.Professional.ID is hoisted to slot.ProfessionalID. The mapping is
// mechanical; tests assert it via a `MappingIsExhaustive` subtest in PR #A.
```

**Rationale — input is a struct of resolved entities, not a DTO:**
The use case has already resolved `Service`, `Professional`, `BusinessProfile`,
`Schedule`, and the per-date `BusinessHoursException` for its own authorization
and active-status concerns (per exploration Q4 / REQ-BV failure modes). Re-resolving
inside the validator would (a) duplicate DB calls, (b) couple the pure validator
to 6 repository interfaces that the caller already holds, and (c) make the validator
untestable without spinning up the full `AvailabilityDeps` graph. Passing resolved
entities keeps the validator pure, fast, and trivially mockable — aligned with
decision D5 (stateless domain services) and the `BookingValidator` spec REQ-BV-2.

### 3.1.1 `domain.BookingValidator` interface — PR #A (new file)

```go
// internal/domain/booking_validator.go (NEW — zero-dep package)
package domain

import "context"

// BookingValidator is the interface that use cases depend on. The concrete
// implementation lives in internal/domain/service/booking_validator.go
// (`*service.BookingValidator`) and satisfies this interface structurally.
//
// Declared in the zero-dep `internal/domain` package so use cases can import
// it without pulling in service-package internals or causing an import cycle
// (use case already imports `internal/domain/repository`).
type BookingValidator interface {
    Validate(ctx context.Context, input ValidateBookingInput) *SemanticError
}

// ValidateBookingInput is the input contract. The concrete struct is defined
// in internal/domain/service (where its dependencies — entity types, the
// narrow BookingOverlapReader — live) and is passed across this interface
// boundary. The interface consumer in internal/domain stays zero-dep.
type ValidateBookingInput struct {
    // Forward-declared; the concrete type lives in internal/domain/service.
    // Use cases populate fields by name; the interface only requires the
    // method signature.
    _ struct{} // placeholder — replaced by import-time re-export from service pkg
}
```

> NOTE TO IMPLEMENTER: the placeholder above is a sketch. The actual interface
> in `internal/domain/booking_validator.go` will be a thin re-export of the
> service-package `ValidateBookingInput` (or an equivalent), declared in the
> zero-dep package via a type alias. The pattern: use case imports
> `internal/domain` (interface) and the input struct via that re-export, so
> the domain package stays free of `entity`/`repository` imports. PR #A's
> TDD cycle will pin the exact form.

### 3.2 `ValidateBookingTimeSlot` (function — package-private) — PR #A

```go
// internal/domain/service/booking_time_validator.go
package service

// BookingOverlapReader is the narrow read interface the helper needs.
// It is a subset of domain/repository.BookingsRepo (only FindOverlapping).
// Defined locally so the helper does NOT depend on the full BookingsRepo.
type BookingOverlapReader interface {
    FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error)
}

// SlotInput is the proposed slot expressed in business-local terms plus the
// durations and identifiers the chain needs to produce localized messages.
type SlotInput struct {
   ProfessionalID   string
    Service          *entity.Service
    Professional     *entity.Professional
    BusinessProfile  *entity.BusinessProfile
    Schedule         *entity.Schedule
    Exception        *entity.BusinessHoursException  // nil == no exception
    Start            time.Time                         // parsed, in business *time.Location
}

// BookingTimeValidatorDeps groups read-side dependencies for the chain.
type BookingTimeValidatorDeps struct {
    Bookings BookingOverlapReader
}

// ValidateBookingTimeSlot runs the 5-step chain in fixed order:
//  1. Past time check
//  2. Business hours check (exception-aware, then JSON weekly schedule)
//  3. Professional schedule check
//  4. Slot-within-combined-hours check
//  5. Overlap check via Bookings.FindOverlapping
// Returns the first *domain.SemanticError, or nil. Short-circuits after step 1
// and after step 4 (past and overlap short-circuit per REQ-BTV-3).
func ValidateBookingTimeSlot(
    ctx context.Context,
    slot SlotInput,
    deps BookingTimeValidatorDeps,
) *domain.SemanticError
```

**Rationale — package-private, not exported:**
`ValidateBookingTimeSlot` is a private implementation detail of the
`internal/domain/service` package. Its sole callers are `BookingValidator.Validate`
and `AvailabilityService.CheckAvailability` (both in-package). Exporting it would
(a) widen the public API surface of the domain service package without a consumer
outside it, (b) freeze the `SlotInput`/`BookingTimeValidatorDeps` shapes against
external callers — violating the Q-DRIFT-1 goal of "single source of truth we can
evolve in one place", and (c) duplicate the validator's contract at two layers.
Keeping it private satisfies Q-DRIFT-1 (single source of truth) while preserving
the freedom to refactor the helper's signature later without a breaking change.

**Rationale — narrow `BookingOverlapReader` subset of `BookingsRepo`:**
The helper only reads overlapping bookings. Defining a local 1-method interface
follows the Go idiom "accept interfaces, return structs" and decouples the helper
from the 10-method `BookingsRepo`. This is the same interface-segregation already
practiced in `availability.go` (which depends on the full `AvailabilityDeps`
struct of repo interfaces). Both the existing `mockBookingsRepo` (in `mocks_test.go`)
and any production `BookingsRepo` implementation satisfy `BookingOverlapReader`
structurally — no new mock required for PR #A.

### 3.3 `AvailabilityService.CheckAvailability` refactor — PR #A

```go
// internal/domain/service/availability.go (MODIFIED — mechanical extract-method)

func (s *AvailabilityService) CheckAvailability(
    ctx context.Context,
    params *CheckAvailabilityParams,
    deps AvailabilityDeps,
) (*CheckAvailabilityResult, error) {
    // ─── Input Resolution (UNCHANGED — lines 63–107 of current file) ───
    svc, err := deps.Services.FindByID(ctx, params.ServiceID)
    // ... active-status checks, profile, loc, StartDatetime parse ...
    // Returns (*CheckAvailabilityResult, error) on lookup/active failures
    // exactly as today.

    // ─── 5-step chain — REPLACED with a single delegate call ───
    if semErr := ValidateBookingTimeSlot(ctx, SlotInput{
        ProfessionalID:  params.ProfessionalID,
        Service:         svc,
        Professional:    pro,
        BusinessProfile: profile,
        Schedule:        schedule,         // now resolved above (was at line 151)
        Exception:       exception,         // now resolved above (was at line 112)
        Start:           startTime,
    }, BookingTimeValidatorDeps{Bookings: deps.Bookings}); semErr != nil {
        return nil, semErr
    }

    return &CheckAvailabilityResult{Available: true}, nil
}
```

- The method **signature, return type, and error codes are UNCHANGED**
  (REQ-AV-1).
- The 16-input regression gate (existing `availability_test.go` subtests)
  passes with **zero diff** to the test file (REQ-AV-2 Regression Gate).
- The resolution block (`Svc`/`pro`/`profile`/`loc`/`startTime`/`exception`/
  `schedule`) stays inline in `CheckAvailability`, because those lookups are
  the use-case-style entity resolution that `AvailabilityService` already owns
  and that the regression gate exercises.

**Rationale — extract-method (not replace):**
The chain is byte-equivalent in observable behaviour: identical step order,
identical error codes, identical Spanish message strings, identical
short-circuit points. The refactor is purely a “move existing lines behind a
function call and pass the same locals as a struct”. Risk is mechanical, mitigated
by the strict regression gate (Section 5).

### 3.4 Use case integration — PR #B (`CreateBooking`), PR #C (`RescheduleBooking`)

```go
// internal/application/usecase/create_booking.go (MODIFIED — PR #B)

type CreateBookingUseCase struct {
    bookings  repository.BookingsRepo
    services  repository.ServicesRepo
    pros      repository.ProfessionalsRepo          // ADDED — needed for entity resolution
    bizProf   repository.BusinessProfileRepo       // ADDED
    bizEx     repository.BusinessHoursExceptionRepo // ADDED
    schedules repository.SchedulesRepo             // ADDED
    validator domain.BookingValidator              // ADDED
}

func NewCreateBookingUseCase(
    bookings, services, professionals, bizProf, bizEx, schedules repository.*,
    validator BookingValidator,      // interface from internal/domain/service
) *CreateBookingUseCase
```

> The five new repos were previously NOT injected because the use case did no
> datetime validation. They are now needed for entity resolution BEFORE the
> validator call. Alternatively (see Open Questions shadowed in proposal §4),
> a single `AvailabilityDeps`-shaped struct could be passed instead — we use
> explicit fields for clarity and constructor signature stability.

- The use case resolves `svc`, `pro`, `profile`, `exception`, `schedule`
  (reusing the same helpers as `AvailabilityService`: `ParseBusinessTimezone`,
  `ParseStartDatetime`) BEFORE calling `validator.Validate`.
- On `validator.Validate` returning a non-nil `*domain.SemanticError`, the
  use case returns it **unchanged** (REQ-BK-10, REQ-BK-11 — no rewrapping to
  `database.ErrConflict`).
- On validator pass, the use case calls `uc.bookings.Create` / `.Reschedule` as today.
- On `domain.ErrConflict` from the repo (TOCTOU race between the validator's read
  and the repo's atomic write), the use case maps to `ErrCodeBookingOverlap`
  exactly as today (REQ-BK-12). This mapping is now correct-by-construction: the
  only remaining path to `domain.ErrConflict` is the atomic overlap guard.

```go
// PR #B pseudo-call inside Execute:
if semErr := uc.validator.Validate(ctx, service.ValidateBookingInput{
    Service: svc, Professional: pro, BusinessProfile: profile,
    ProfessionalSchedule: schedule, Exception: exception,
    NewStart: input.StartTime, Bookings: uc.bookings,
}); semErr != nil {
    return nil, semErr             // propagate unchanged
}
// proceed to uc.bookings.Create(ctx, booking) as today
```

- The domain interface `BookingValidator` is declared in `internal/domain/booking_validator.go` (the zero-dep package — see §3.1.1) and is satisfied structurally by `*service.BookingValidator`. The use case imports the interface from `internal/domain`, NOT from `internal/domain/service`. This avoids the import cycle (use case → domain/service → domain/repository): the use case already depends on `internal/domain` and `internal/domain/repository`; adding a new import from `internal/domain` is a strict DAG addition, NOT a back-edge. (The §3.1.1 placeholder will be resolved to a type-alias re-export during PR #A's TDD cycle; the package dependency is settled.)

## Section 4: Test Strategy

Strict TDD (test runner: `go test -v -race ./...`). All tests are table-driven,
hand-rolled mocks only — **no `sqlmock`, no real SQLite**.

### 4.1 PR #A — domain service layer

**`ValidateBookingTimeSlot`** (`booking_time_validator_test.go`):

| Subtest | Scenario | Want code | Short-circuit? |
|---|---|---|---|
| `past_time` | `Start < time.Now()` | `ErrCodeSlotInPast` | yes (no overlap query) |
| `business_closed_exception` | closed-day exception | `ErrCodeBusinessClosed` | yes |
| `business_closed_json_fallback` | empty weekly JSON | `ErrCodeBusinessClosed` | yes |
| `professional_not_working` | no schedule row | `ErrCodeProfessionalNotWorking` | yes |
| `slot_ends_after_close` | `slotEnd>effectiveClose` | `ErrCodeSlotOutOfHours` | yes |
| `slot_starts_before_business_open` | `slotStart<businessOpen` | `ErrCodeSlotOutOfHours` | yes |
| `slot_starts_before_professional_start` | `slotStart<proStart` | `ErrCodeSlotOutOfHours` | yes |
| `overlap_detected` | mocked FindOverlapping returns 1 | `ErrCodeBookingOverlap` | — |
| `all_pass` | valid slot | `nil` | n/a |

Mock: **reuse existing `mockBookingsRepo`** from `mocks_test.go` (extend
`OnFindOverlapping` — already present). No new mock boilerplate.

**`BookingValidator.Validate`** (`booking_validator_test.go`):

Same 9-row matrix driven through `Validate` (which delegates to the helper), plus:

| Extra | Scenario | Want |
|---|---|---|
| `nil_ctx_panic_safe` | `ctx = nil` | the helper must short-circuit before any DB call; assert no `FindOverlapping` call is made for past slots |
| `inactive_service_not_validated` | `Service.Active=false` | helper proceeds; validator does NOT return `ErrCodeServiceNotActive` |
| `inactive_professional_not_validated` | `Professional status=inactive` | validator proceeds past active check |
| `first_error_short_circuits` | past AND overlap | returns `ErrCodeSlotInPast`; mock `OnFindOverlapping` is NEVER called (assert via a call-count mock) |

**Regression gate (hard merge gate for PR #A):**

```bash
go test -v -race -run TestCheckAvailability ./internal/domain/service/
```

MUST pass **all existing `t.Run` subtests with ZERO diff** to
`availability_test.go`. The 15 subtests of `TestCheckAvailability` (happy_path
through professional_inactive) cover every step of the chain, every active-status
branch, and every lookup-error path — they cross-validate the refactor. Adding
`TestHHMMToMinutes` brings the file to ~16 `t.Run` invocations; both must pass.

### 4.2 PR #B — `CreateBookingUseCase`

Create `internal/application/usecase/mocks_test.go` (or extend if present) with
`mockBookingValidator` (function-table pattern, mirroring `mocks_test.go`):

```go
type mockBookingValidator struct {
    OnValidate func(context.Context, service.ValidateBookingInput) *domain.SemanticError
}
func (m *mockBookingValidator) Validate(ctx context.Context, in service.ValidateBookingInput) *domain.SemanticError {
    return m.OnValidate(ctx, in)
}
```

Table-driven subtests for `TestCreateBookingUseCase_Execute`:

| # | Name | Validator ret | Repo ret | Want code |
|---|---|---|---|---|
| 1 | happy_path | nil | nil | result.BookingID != "" |
| 2 | past_slot | `ErrCodeSlotInPast` | (not called) | `ErrCodeSlotInPast` |
| 3 | business_closed | `ErrCodeBusinessClosed` | (not called) | `ErrCodeBusinessClosed` |
| 4 | professional_not_working | `ErrCodeProfessionalNotWorking` | — | same |
| 5 | slot_out_of_hours | `ErrCodeSlotOutOfHours` | — | same |
| 6 | overlap | `ErrCodeBookingOverlap` | — | same |
| 7 | service_not_active (use case path) | nil | — | `ErrCodeServiceNotActive` (use case still owns this check) |
| 8 | toctou_repo_overlap | nil | `domain.ErrConflict` | `ErrCodeBookingOverlap` (via repo path) |

Subtests 2–6 prove the use case propagates validator errors unchanged
(REQ-BK-10, REQ-BK-11). Subtest 8 is the TOCTOU guard (REQ-BK-12) and proves
the repo atomic check stays reachable.

### 4.3 PR #C — `RescheduleBookingUseCase`

Same 8-row matrix adapted to the reschedule shape (existing booking must load first;
the matrix runs after `CanReschedule` passes). The TOCTOU subtest asserts the
`Reschedule` repo path can still return `ErrConflict` while the validator passed.

## Section 5: Implementation Order (3 PRs)

### PR #A — BookingValidator + helper + AvailabilityService refactor

- **Branch**: `feat/feat-booking-validator-apply-pr-a`
- **Estimated LOC**: ~560 (within the user-approved `delivery_strategy=ask-on-risk`
  exception; 400 default + 160 helper extraction)
- **Pre-flight**: `go fmt ./...`, `go vet ./...`, `go build -o /dev/null ./...`,
  `golangci-lint run ./...`, `go test -v -race ./...` — all green
- **GGA**: pass (auto on commit)
- **Judgment Day**: 2 blind judges; one CRITICAL → fix → re-judge
- **Success criteria**:
  1. All existing `availability_test.go` subtests pass with **zero diff** to the test file.
  2. New `booking_time_validator_test.go` and `booking_validator_test.go` pass `-race`.
  3. `BookingValidator.Validate` and `AvailabilityService.CheckAvailability` produce
     the same `*domain.SemanticError.Code` for the same input (cross-validation test).
  4. No `internal/repository/` or `database/sql` import in `internal/domain/service/`.
- **Risk + mitigation**: the `availability.go` refactor could regress the gate.
  Mitigation: execute the refactor as a byte-equivalent extract-method — the chain
  body moves verbatim into `ValidateBookingTimeSlot`, `CheckAvailability` becomes
  the resolution block + a single delegate call. Intermediate state (helper exists,
  `CheckAvailability` still inlines the chain) is committed privately to confirm the
  gate passes before the delegate swap.

### PR #B — CreateBooking integration

- **Branch**: `feat/feat-booking-validator-apply-pr-b`
- **Estimated LOC change**: ~80 (±30 production + ~50 test)
- **Pre-flight**: full pipeline green
- **GGA + Judgment Day**: required
- **Success criteria**: 8 table-driven subtests pass; the gap (no `ErrCodeSlotInPast`
  for `CreateBooking`) is closed; `go test -race ./...` green.
- **Risk + mitigation**: constructor-arity change breaks `main.go` and other callers.
  Mitigation: `main.go` is updated in the same PR (P4 wiring preview — see Section 7).

### PR #C — RescheduleBooking integration + cleanup

- **Branch**: `feat/feat-booking-validator-apply-pr-c`
- **Estimated LOC change**: ~80
- **Success criteria**: reschedule gap closed; 8 subtests pass; final cleanup
  (any helper function renamed/consolidated, single `BookingValidator` instance
  shared between both use cases).
- **Risk + mitigation**: same as PR #B.

## Section 6: Migration / Compatibility

- **No DB migration**: SQLite schema unchanged; `internal/repository/bookings.go`
  unmodified (REQ-BV failure modes / Decisión 11 — atomic overlap guard stays).
- **No schema change**: `bookings`, `business_profile`, `schedule`,
  `business_hours_exception` tables untouched.
- **No auth change**: `internal/auth/`, `auth.RequireAuthenticated`,
  `AuthorizeBookingAccess` untouched.
- **No public API change**: MCP tool inputs, outputs, and error codes are
  byte-identical. The MCP tool layer wraps `*domain.SemanticError` by `Code`
  — all codes it emits already exist.
- **Only behavioural change**: `CreateBooking` and `RescheduleBooking` now return
  error codes that previously reached the user only via `CheckAvailability`:
  `ErrCodeSlotInPast`, `ErrCodeBusinessClosed`, `ErrCodeProfessionalNotWorking`,
  `ErrCodeSlotOutOfHours`. (The taxonomy is unchanged — no new codes added; the
  codes are simply emitted on new paths, which is the entire point of the change.)
- **Rollback**: each PR is independently revertible via `git revert <merge>`.
  No data rollback is required because there is no schema/data change. Worst-case
  full rollback restores `main` to commit `11f237f` (post-P3.3a baseline).

## Section 7: Dependency Injection (preview, full DI in P4)

- `BookingValidator` is stateless → **single shared instance** created once in
  `cmd/mcp-server/main.go` (P4 wiring) and passed to both use case constructors.
- **PR #A does NOT touch `main.go`**: the validator has no production callers
  yet. Tests construct it via `NewBookingValidator()` directly.
- **PR #B/C**: `main.go` constructs `NewBookingValidator()` once and passes it
  to `NewCreateBookingUseCase(...)` / `NewRescheduleBookingUseCase(...)`.
- **`AvailabilityService` deps**: post-refactor, `CheckAvailability(ctx, params, deps)`
  still receives an `AvailabilityDeps` struct (6 repos) — the resolution block
  remains in the method. The helper's `BookingTimeValidatorDeps` is narrower
  (1 repo) and is built INSIDE `CheckAvailability` from the existing `deps.Bookings`.
  **`AvailabilityDeps` shape is unchanged** — callers (only `CheckAvailabilityUseCase`)
  compile without modification (REQ-AV-1).
- **Use case new repo deps** (Section 3.4): to keep PR #B/C review scoped, the use
  case constructor signature gains the 5 new repo params plus the validator. If
  the team prefers a single struct, an alternative is to pass a `domain.BookingResolution`
  helper struct; **not in this design** — explicit params keep the constructor
  signature discoverable. (P4 wiring will populate them from the existing DI graph;
  all 6 repos are already instantiated in `main.go` for the other use cases.)

## Section 8: Risks

| # | Risk | Severity | Mitigation |
|---|------|----------|------------|
| R1 | Repo atomic overlap check becomes unreachable if the validator always catches overlap first → defense-in-depth silently dead | Low | REQ-BK-12 keeps the repo guard explicitly; PR #B/#C TOCTOU subtest (#8) injects a validator-pass-then-repo-conflict sequence and asserts the repo path still fires. |
| R2 | 14 new subtests (7 codes × 2 use cases) — wide test surface | Medium | Table-driven shared mock setup; matrix encoded as `[]struct{ name; validatorRet; repoRet; wantCode }`. 1 mock, 8 rows per use case. |
| R3 | Drift between `AvailabilityService.CheckAvailability` and `BookingValidator.Validate` | RESOLVED | Q-DRIFT-1: shared `ValidateBookingTimeSlot` in PR #A — both services call the same private helper. No drift possible by construction. |
| R4 | 2 new mock interfaces (`BookingValidator` for use cases; `BookingOverlapReader` for helper) | Low | `BookingOverlapReader` is satisfied structurally by the existing `mockBookingsRepo` and any `domain.BookingsRepo` — no extra mock code for PR #A. `mockBookingValidator` in PR #B/#C reuses the function-table pattern from `mocks_test.go`. |
| R5 | PR #A LOC budget (560 vs 400 default) | Accepted | User-approved via `delivery_strategy=ask-on-risk`. Per-decision 5, the helper extraction (Q-DRIFT-1) grows PR #A from ~370 to ~560; the cost is the price of single-source-of-truth from day 1. |
| R6 (added) | `availability.go` refactor regresses the 15-subtest gate | Medium | Mechanical byte-equivalent extract-method; intermediate private commit (helper exists, `CheckAvailability` still inlines chain) confirms the gate is green BEFORE the delegate swap; gate is a hard merge block. |
| R7 (resolved) | Import cycle: use case → domain/service → domain/repository (use case already imports the latter) | Low → Resolved | The `BookingValidator` interface is declared in `internal/domain/` (zero-dep package — see §3.1.1) and the use case imports it from there. The use case does NOT import `internal/domain/service` directly; it imports the interface from `internal/domain`. The `*service.BookingValidator` struct satisfies the interface structurally. No cycle possible. |

## Section 9: Open Questions

None — all 5 user decisions are resolved in proposal §Architectural Decisions (1–5),
and Q-DRIFT-1 is RESOLVED (shared helper from PR #A, decision 5). The two
shadow-decisions surfaced during design (use case constructor arity in Section 3.4,
and the import-cycle fallback in Section 3.4 / R7) are implementation-time
micro-decisions with documented defaults — they do NOT block the spec or tasks
phase.

## Section 10: References

### Inputs read for this design
- `openspec/changes/feat-booking-validator-service/proposal.md` (134 LOC, Accepted)
- `openspec/changes/feat-booking-validator-service/specs/booking-validator/spec.md` (6 reqs, 12 scenarios)
- `openspec/changes/feat-booking-validator-service/specs/booking-time-validator/spec.md` (5 reqs, 8 scenarios)
- `openspec/changes/feat-booking-validator-service/specs/bookings/spec.md` (delta — 4 added reqs, 5 scenarios)
- `openspec/changes/feat-booking-validator-service/specs/availability/spec.md` (delta — 2 added reqs, 5 scenarios + Regression Gate)
- `openspec/changes/feat-booking-validator-service/exploration.md` (249 LOC)

### Existing code read for design context
- `internal/domain/service/availability.go` (237 LOC — the 5-step chain seed)
- `internal/domain/service/datetime_helpers.go` (44 LOC — `ParseBusinessTimezone`, `ParseStartDatetime`, `hhmmToMinutes`)
- `internal/domain/service/mocks_test.go` (100 LOC — function-table mock pattern to reuse)
- `internal/domain/service/availability_test.go` (239 LOC — 15 `t.Run` subtests + `TestHHMMToMinutes`; the regression gate)
- `internal/application/usecase/create_booking.go` (95 LOC — current state, modified in PR #B)
- `internal/application/usecase/reschedule_booking.go` (72 LOC — current state, modified in PR #C)
- `internal/application/usecase/check_availability.go` (UNCHANGED — reference)
- `internal/domain/repository/bookings.go` (46 LOC — 10-method interface, `FindOverlapping` for the helper dep)
- `internal/domain/errors.go` (74 LOC — `SemanticError` + 7 `ErrCode*` constants, UNCHANGED)

### Issue / PR references
- Issue #22 — RescheduleBookingUseCase delegates datetime validation to the repo
- Issue #23 — Datetime validation is scattered across 3 places
- PR #33 — refactor-clean-architecture P3.2 (clean BookingsRepo interface)
- PR #34, #35 — refactor-clean-architecture intermediate
- PR #36 — refactor-clean-architecture P3.3a (repo `CheckAvailability` duplicate removed, `AvailabilityService` is sole source of the 5-step chain)

### Existing capabilities in `openspec/specs/`
- `openspec/specs/bookings/spec.md` — existing `bookings` capability (MODIFIED by this change per delta)
- `openspec/specs/availability/spec.md` — existing `availability` capability (public contract preserved per delta)

### Sibling design docs (style reference)
- `openspec/changes/refactor-clean-architecture/design.md` — matched conventions: Layer Architecture header, ASCII diagram, Decisiones (D1–D10) translated to per-decision rationale; reuse of D5 (stateless domain services), D9 (manual DI in `cmd/mcp-server/main.go`), D10 (each phase leaves `go test -race ./...` green).
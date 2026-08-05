# Spec: booking-validator

> Reference: `openspec/changes/feat-booking-validator-service/proposal.md` Decision 1, Q-DRIFT-1
> Change: feat-booking-validator-service
> Status: NEW

## Purpose

`BookingValidator` is a stateless domain service in `internal/domain/service/` that orchestrates datetime validation for booking mutations. It delegates the 5-step chain to the shared `ValidateBookingTimeSlot` helper and returns the first `*domain.SemanticError` encountered. Use cases call it with already-resolved entities before dispatching to the repository.

## Requirements

### REQ-BV-1: Stateless Go struct in `internal/domain/service/booking_validator.go`

The `BookingValidator` type SHALL be a stateless Go struct living in `internal/domain/service/booking_validator.go`. It MUST NOT hold `*sql.DB`, SQL connections, or mutable state.

#### Scenario: Constructor returns a usable validator

- GIVEN a freshly constructed `BookingValidator`
- WHEN a test calls its `Validate` method with valid inputs
- THEN the call MUST succeed without initializing a database

### REQ-BV-2: `Validate(ctx, input ValidateBookingInput) *domain.SemanticError`

The service SHALL expose a `Validate(ctx context.Context, input ValidateBookingInput) *domain.SemanticError` method. The input MUST carry the proposed slot and all entity dependencies already resolved by the use case.

#### Scenario: Valid slot returns nil

- GIVEN a proposed slot within business hours, professional schedule, and no overlap
- WHEN `Validate` is called
- THEN it MUST return `nil`

### REQ-BV-3: Delegation to `ValidateBookingTimeSlot`

The service SHALL call `ValidateBookingTimeSlot(ctx, input.Slot, deps)` where `deps` contains the already-resolved service, professional, business profile, schedules, and existing bookings interface.

#### Scenario: Service delegates and returns helper result

- GIVEN a proposed slot that fails the professional-schedule step
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeProfessionalNotWorking}`

### REQ-BV-4: Return the first error from the chain

If `ValidateBookingTimeSlot` returns a `*domain.SemanticError`, the service MUST return it unchanged. If the helper returns `nil`, the service MUST return `nil`.

#### Scenario: Slot in the past returns `ErrCodeSlotInPast`

- GIVEN a proposed `start_datetime` before the current time
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeSlotInPast}`

#### Scenario: Business closed returns `ErrCodeBusinessClosed`

- GIVEN a proposed slot on a date when the business is closed
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeBusinessClosed}`

#### Scenario: Professional not working returns `ErrCodeProfessionalNotWorking`

- GIVEN a proposed slot on a weekday when the professional has no schedule
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeProfessionalNotWorking}`

#### Scenario: Slot out of hours returns `ErrCodeSlotOutOfHours`

- GIVEN a proposed slot that extends beyond the combined business and professional hours
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeSlotOutOfHours}`

#### Scenario: Overlap returns `ErrCodeBookingOverlap`

- GIVEN a proposed slot that overlaps an existing non-cancelled booking
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeBookingOverlap}`

#### Scenario: Inactive service or professional is not validated by this service

- GIVEN a proposed slot whose resolved service or professional is inactive
- WHEN `Validate` is called
- THEN it MUST NOT return `ErrCodeServiceNotActive` or `ErrCodeProfessionalNotActive`
- AND it MUST proceed with the 5-step chain

#### Scenario: Multiple failures short-circuit at the first

- GIVEN a proposed slot that is both in the past and overlaps another booking
- WHEN `Validate` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeSlotInPast}` and MUST NOT return `ErrCodeBookingOverlap`

### REQ-BV-5: Zero `database/sql` dependency

The `internal/domain/service/booking_validator.go` file MUST NOT import `database/sql` or any package that requires a SQL driver. Validation MUST depend only on repository interfaces and domain types.

#### Scenario: Static analysis confirms no SQL import

- GIVEN the source file is parsed
- WHEN `imports` are enumerated
- THEN `database/sql` MUST NOT be present

### REQ-BV-6: Testable with pure mocks

Tests for `BookingValidator` MUST use hand-rolled interface mocks and MUST NOT use `sqlmock` or a real database.

#### Scenario: Full matrix runs without sqlmock

- GIVEN a table-driven test with mocked `Bookings.FindOverlapping` and resolved entities
- WHEN all 7 invalid scenarios plus the happy path execute
- THEN every subtest MUST pass without importing `github.com/DATA-DOG/go-sqlmock`

## Failure Modes

`BookingValidator` does NOT check service/professional active status, authorization, or entity resolution. Those responsibilities remain in the use case per exploration Q4. The service assumes all entities in `ValidateBookingInput` are already resolved and valid for the tenant.

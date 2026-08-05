# Spec: booking-time-validator

> Reference: `openspec/changes/feat-booking-validator-service/proposal.md` Decision 5 (Q-DRIFT-1)
> Change: feat-booking-validator-service
> Status: NEW

## Purpose

`ValidateBookingTimeSlot` is a package-private pure helper in `internal/domain/service/booking_time_validator.go` that executes the 5-step datetime validation chain. It performs no I/O, holds no state, and is shared by `BookingValidator.Validate` and `AvailabilityService.CheckAvailability`.

## Requirements

### REQ-BTV-1: Package-private pure helper signature

The helper SHALL be declared as `func ValidateBookingTimeSlot(ctx context.Context, slot SlotInput, deps Deps) *domain.SemanticError` in `internal/domain/service/booking_time_validator.go`. It MUST accept a proposed slot and resolved dependencies, and MUST return either the first semantic error or `nil`.

#### Scenario: All checks pass

- GIVEN a slot that satisfies all five validation steps
- WHEN `ValidateBookingTimeSlot` is called
- THEN it MUST return `nil`

### REQ-BTV-2: Execute the 5 steps in deterministic order

The helper SHALL run the following steps IN ORDER:
1. Past time check
2. Business hours check (exception-aware, then JSON weekly schedule)
3. Professional schedule check
4. Slot-within-combined-hours check — uses `SlotInput.Service.Duration` to compute the proposed slot's end time, then verifies the slot fits within the combined business + professional hours window. The `Service` field is therefore a required input for step 4; if the use case passes `Service == nil` or `Service.Duration <= 0`, the helper MUST panic (programmer error — the contract requires the use case to populate this field before calling). This is NOT a `*domain.SemanticError` because a missing duration is a contract violation, not a business validation failure.
5. Overlap check via `Bookings.FindOverlapping`

#### Scenario: Past time returns first

- GIVEN a proposed `start_datetime` before the current time
- WHEN `ValidateBookingTimeSlot` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeSlotInPast}` and MUST NOT proceed to business hours

#### Scenario: Business closed returns before professional schedule

- GIVEN a slot on a date with a closed business exception
- WHEN `ValidateBookingTimeSlot` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeBusinessClosed}` before checking the professional schedule or overlap

#### Scenario: Professional not working returns before slot-within-hours

- GIVEN a slot on a weekday when the professional has no schedule
- WHEN `ValidateBookingTimeSlot` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeProfessionalNotWorking}` before checking slot fit

#### Scenario: Slot out of hours returns before overlap

- GIVEN a slot that extends beyond the combined business and professional hours
- WHEN `ValidateBookingTimeSlot` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeSlotOutOfHours}` before invoking `Bookings.FindOverlapping`

### REQ-BTV-3: Short-circuit on first error

The helper MUST return after the first failing step and MUST NOT execute subsequent steps.

#### Scenario: Past slot does not trigger overlap query

- GIVEN a proposed slot in the past that also overlaps an existing booking
- WHEN `ValidateBookingTimeSlot` is called
- THEN it MUST return `ErrCodeSlotInPast` and MUST NOT invoke `Bookings.FindOverlapping`

### REQ-BTV-4: Depend on `Bookings.FindOverlapping` interface

The overlap step MUST use the `Bookings.FindOverlapping` method from the `domain.BookingsRepo` interface. It MUST NOT accept `*sql.DB` or execute raw SQL directly.

#### Scenario: Overlap uses mocked repository

- GIVEN a mocked `Bookings` implementation that returns an overlapping booking
- WHEN the overlap step runs
- THEN it MUST return `ErrCodeBookingOverlap` without touching a database

### REQ-BTV-5: Testable without a database

The helper MUST be unit-testable using only hand-rolled mocks and pure inputs.

#### Scenario: Exhaustive matrix runs offline

- GIVEN a table-driven test covering past, business hours, professional schedule, slot-within-hours, and overlap cases
- WHEN all subtests execute
- THEN none MUST require `sqlmock` or an in-memory SQLite instance

## Failure Modes

Short-circuit semantics make error precedence deterministic: earlier steps in the chain suppress later ones. Callers MUST NOT rely on a later error being returned when an earlier step fails.

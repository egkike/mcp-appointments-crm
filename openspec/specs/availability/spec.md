# Delta: availability

> Reference: `internal/domain/service/availability.go`; `openspec/changes/feat-booking-validator-service/proposal.md` Decision 5
> Change: feat-booking-validator-service
> Status: DELTA

## ADDED Requirements

### ADDED REQ-AV-1: Public contract of `AvailabilityService.CheckAvailability` is unchanged

The signature, return type, and every documented error code of `AvailabilityService.CheckAvailability` MUST remain identical after this change.

#### Scenario: Existing callers compile without modification

- GIVEN an existing caller of `AvailabilityService.CheckAvailability`
- WHEN the project is rebuilt after PR #A
- THEN the caller MUST compile without source changes

### ADDED REQ-AV-2: Internal 5-step chain replaced by `ValidateBookingTimeSlot`

Internally, `AvailabilityService.CheckAvailability` SHALL execute the 5-step chain by calling `ValidateBookingTimeSlot(ctx, slot, deps)`. The behavior, error codes, and short-circuit order MUST be byte-identical to the pre-refactor implementation.

#### Scenario: Business closed returns same error

- GIVEN a slot on a closed business day
- WHEN `AvailabilityService.CheckAvailability` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeBusinessClosed}` with the same message as before the refactor

#### Scenario: Slot in the past returns same error

- GIVEN a slot in the past
- WHEN `AvailabilityService.CheckAvailability` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeSlotInPast}` with the same message as before the refactor

#### Scenario: Overlap returns same error

- GIVEN a slot that overlaps an existing non-cancelled booking
- WHEN `AvailabilityService.CheckAvailability` is called
- THEN it MUST return `&SemanticError{Code: ErrCodeBookingOverlap}` with the same message as before the refactor

#### Scenario: Happy path returns available

- GIVEN a slot that passes all five steps
- WHEN `AvailabilityService.CheckAvailability` is called
- THEN it MUST return `nil` error and indicate the slot is available

## Regression Gate

All 16 existing subtests in `availability_test.go` MUST pass unmodified after PR #A. No test assertions, table entries, or expected error messages in that file may change. This is a hard gate for merging PR #A.

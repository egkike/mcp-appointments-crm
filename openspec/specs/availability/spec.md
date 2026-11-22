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

### ADDED REQ-AV-3: Read-only date-range slot search (`find_available_slots`)

> **Provenance**: `odd/tasks/feat-find-available-slots.md` (ODD feature doc, issue #241). Recorded here in place per the owner's decision to keep `openspec/specs/` as the normative home without reviving the SDD change pipeline (no `openspec/changes/<change>/` entry). This section does not restate or amend REQ-AV-1/REQ-AV-2.

The MCP tool `find_available_slots` MUST answer which slots are free for a service over an inclusive date range, in one call, without duplicating the availability rule.

Availability MUST be evaluated by `service.ValidateBookingTimeSlot` — the same helper `CheckAvailability` delegates to (REQ-AV-2) — injected with a `BookingOverlapReader` backed by the range's bookings already read in memory. The implementation MUST NOT re-implement the business-hours, exception, schedule, window or overlap checks.

`find_available_slots` **filters**; it does not report. A `*domain.SemanticError` returned by the chain means "this slot is not available" and MUST NOT reach the client. This is the deliberate difference from `check_availability`, which reports the failing reason.

#### Scenario: No slots in the range returns an empty list, not an error
- GIVEN a service, a business-local range with no free slot
- WHEN `find_available_slots` is called
- THEN it MUST return `{slots: [], total_candidates: 0, truncated: false}` with no error

#### Scenario: A closed-hours exception removes that date
- GIVEN a `business_hours_exceptions` row with `is_closed=true` for a date inside the range
- WHEN `find_available_slots` is called
- THEN no slot MUST be produced for that date

#### Scenario: Overlapping bookings are filtered, not reported
- GIVEN an existing non-cancelled booking covering a candidate slot
- WHEN `find_available_slots` is called
- THEN that slot MUST be absent from `slots`
- AND no `ErrCodeBookingOverlap` MUST reach the client

#### Scenario: The range is read once
- GIVEN a range spanning several days and several professionals
- WHEN `find_available_slots` is called
- THEN the bookings for the whole range MUST be read with a single query

#### Scenario: Range and limit bounds are enforced
- GIVEN `to - from` greater than 14 inclusive days, or a `limit` above 50
- WHEN `find_available_slots` is called
- THEN it MUST return a semantic error and MUST NOT return a slot list

#### Scenario: Response carries no client identity
- GIVEN slots filtered out by existing client bookings
- WHEN `find_available_slots` is called
- THEN `slots` MUST contain only `start_datetime`, `professional_id` and `professional_name`

## Regression Gate

All 16 existing subtests in `availability_test.go` MUST pass unmodified after PR #A. No test assertions, table entries, or expected error messages in that file may change. This is a hard gate for merging PR #A.

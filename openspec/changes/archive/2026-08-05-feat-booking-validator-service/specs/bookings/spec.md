# Delta: bookings

> Reference: `openspec/specs/bookings/spec.md`; `openspec/changes/feat-booking-validator-service/proposal.md`
> Change: feat-booking-validator-service
> Status: DELTA

## ADDED Requirements

### ADDED REQ-BK-9: Use cases invoke `BookingValidator.Validate` before repo dispatch

`CreateBookingUseCase` and `RescheduleBookingUseCase` MUST invoke `BookingValidator.Validate(ctx, input)` BEFORE dispatching to `BookingsRepo.Create` or `BookingsRepo.Reschedule`.

#### Scenario: Validator rejects past slot before repo is reached

- GIVEN `CreateBookingUseCase` receives a `start_datetime` in the past
- WHEN the use case runs
- THEN `BookingValidator.Validate` MUST be called
- AND `BookingsRepo.Create` MUST NOT be invoked

### ADDED REQ-BK-10: Use cases propagate validator semantic errors unchanged

When `BookingValidator.Validate` returns a `*domain.SemanticError`, both use cases MUST return that error to the caller without modification.

#### Scenario: Validator returns `ErrCodeBusinessClosed`

- GIVEN `RescheduleBookingUseCase` receives a slot on a closed business day
- WHEN `BookingValidator.Validate` returns `&SemanticError{Code: ErrCodeBusinessClosed}`
- THEN the use case MUST return the same `*SemanticError` with the same code and message

#### Scenario: Use case emits `ErrCodeServiceNotActive` from its own active-status check (validator NOT called)

- GIVEN `CreateBookingUseCase` or `RescheduleBookingUseCase` resolves an inactive service during entity resolution
- WHEN the use case's pre-validator active-status check fires
- THEN the use case MUST return `&SemanticError{Code: ErrCodeServiceNotActive}` directly
- AND the use case MUST NOT call `BookingValidator.Validate` (the validator does NOT check service active status — see booking-validator/spec.md REQ-BV-4 failure modes; the validator proceeds past active checks)

### ADDED REQ-BK-11: `*domain.SemanticError` MUST NOT be rewrapped as `domain.ErrConflict`

Both use cases MUST NOT catch a `*domain.SemanticError` from the validator and map it to `domain.ErrConflict`.

#### Scenario: Validator overlap is returned directly

- GIVEN `BookingValidator.Validate` returns `&SemanticError{Code: ErrCodeBookingOverlap}`
- WHEN the use case handles the error
- THEN it MUST return the semantic error directly and MUST NOT return `domain.ErrConflict`

### ADDED REQ-BK-12: Repo atomic overlap check remains as defense-in-depth

The repository's atomic `INSERT ... WHERE NOT EXISTS` / `UPDATE ... WHERE NOT EXISTS` overlap check REMAINS. If the validator passes and the repo's atomic guard fires due to a TOCTOU race, the use case maps `domain.ErrConflict` to `ErrCodeBookingOverlap` as today.

#### Scenario: Validator passes but repo atomic check fires

- GIVEN `BookingValidator.Validate` returns `nil`
- AND `BookingsRepo.Create` returns `domain.ErrConflict` due to a concurrent overlapping insert
- WHEN `CreateBookingUseCase` handles the repo error
- THEN it MUST return `&SemanticError{Code: ErrCodeBookingOverlap}`

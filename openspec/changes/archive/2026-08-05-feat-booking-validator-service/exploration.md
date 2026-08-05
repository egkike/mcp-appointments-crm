# Exploration: BookingValidator Domain Service

> **Change**: feat-booking-validator-service
> **Status**: Explored (with post-hoc drift notes)
> **Date**: 2026-08-03
> **Related**: Issues #22, #23

> **⚠️ DRIFT NOTE (added 2026-08-03 after Judgment Day on the SDD docs)**:
> This exploration was written before `refactor-clean-architecture` P3.3a (commit `11f237f`, PR #36) merged. The references below to "the repo duplicate", "Phase 3 hasn't been completed", and the pre-P3 `BookingsRepo` method names are now historical record. The current state is:
> - **Repo duplicate removed**: `BookingsRepo.CheckAvailability` (the 244-LOC SQL duplicate) is GONE as of P3.3a. `AvailabilityService.CheckAvailability` is the SOLE source of the 5-step chain.
> - **BookingsRepo interface is clean**: 10 methods (`Create`, `FindByID`, `Update`, `Cancel`, `Reschedule`, `FindOverlapping`, `FindByStaffAndRange`, `ListBookingsForRange`, `SearchByNotes`, `UpdateStatus`) per the post-P3.2 contract.
> - **Use cases call post-P3 names**: `CreateBookingUseCase` calls `bookings.Create(ctx, booking)`, `RescheduleBookingUseCase` calls `bookings.Reschedule(ctx, id, start, end)`.
> For the up-to-date surface area, see `openspec/changes/feat-booking-validator-service/proposal.md` §Risks and the current state of `internal/repository/bookings.go`.

## 1. Executive Summary

This change introduces a `BookingValidator` domain service in `internal/domain/service/` that owns all booking-datetime validation (past, business hours, professional schedule, slot within hours, overlap). Today, this logic is scattered across three places: the `AvailabilityService` domain service (5-step chain, used only by `CheckAvailabilityUseCase`), the SQL `BookingsRepo` `CheckAvailability` method (a near-duplicate of the domain service), and the `BookingsRepo` `CreateBooking`/`RescheduleBooking` methods (overlap-only via atomic SQL). Two use cases — `CreateBookingUseCase` and `RescheduleBookingUseCase` — perform NO datetime validation themselves; they delegate to the repo and receive only overlap errors back. This means a booking in the past or outside business hours would pass through the use case undetected until it hits the (non-existent) repo check.

**Recommendation**: Option C — extract a new `BookingValidator` in `internal/domain/service/booking_validator.go` that consolidates the 5-step chain currently in `AvailabilityService`, deletes the duplicate in the repo, and is called by all three use cases (`CheckAvailability`, `CreateBooking`, `Reschedule`) BEFORE dispatching to the repo. The repo retains only its atomic overlap check as defense-in-depth (per design Decisión 11). This is consistent with the hexagonal architecture, design decisions D5/D9, and the domain service contract C1 already established in `refactor-clean-architecture`.

**Critical dependency**: This change MUST be sequenced AFTER `refactor-clean-architecture` Phase 3.2 (repo interface conformance) completes. The current `BookingsRepo` does not implement `domain/repository.BookingsRepo` — methods are named `CreateBooking`/`RescheduleBooking`/`GetBooking` instead of `Create`/`Reschedule`/`FindByID`. The validator depends on the post-P3 interface contract.

## 2. Current State of Datetime Validation

### 2.1 Validation Matrix

| Use case | Validates datetime? | Where? | What's checked | Error codes returned | Test coverage of validation |
|---|---|---|---|---|---|
| `CheckAvailabilityUseCase` | YES (via domain service) | `domain/service/availability.go:56-237` (5-step chain via repo interfaces) | 3a. Business hours (exception/JSON), 3b. Professional schedule, 3c. Slot within combined hours, 3d. Overlap (via `Bookings.FindOverlapping`), 3e. Not in past | `ErrCodeBusinessClosed`, `ErrCodeProfessionalNotWorking`, `ErrCodeSlotOutOfHours`, `ErrCodeBookingOverlap`, `ErrCodeSlotInPast`, `ErrCodeServiceNotActive`, `ErrCodeProfessionalNotActive` | `check_availability_test.go` (272 LOC, 9 subtests) — mocks the `AvailabilityChecker` interface |
| `CreateBookingUseCase` | NO (delegated to repo via domain interface) | `application/usecase/create_booking.go:88` → `bookingsRepo.Create(ctx, booking)` | Overlap only (atomic `INSERT ... WHERE NOT EXISTS`) — `repository/bookings.go:102-116` | `ErrCodeBookingOverlap` (from `domain.ErrConflict` mapping, line 89-90) | `create_booking_test.go` (336 LOC, 9 subtests) — does NOT test datetime validation; mocks `CreateFn` |
| `RescheduleBookingUseCase` | NO (delegated to repo via domain interface) | `application/usecase/reschedule_booking.go:57` → `bookingsRepo.Reschedule(ctx, id, start, end)` | Overlap only (atomic `UPDATE ... WHERE NOT EXISTS`) — `repository/bookings.go:306-317` | `ErrCodeBookingOverlap` (from `domain.ErrConflict` mapping, line 58-59) | `reschedule_booking_test.go` (315 LOC, 9 subtests) — does NOT test datetime validation; mocks `RescheduleFn` |
| `CancelBookingUseCase` | N/A | No datetime input | N/A | N/A | N/A |

### 2.2 The Duplicate: Two CheckAvailability Implementations

There are TWO implementations of the 5-step validation chain, differing only in data access:

| Implementation | File | Lines | Deps | Testable without DB? |
|---|---|---|---|---|
| **Domain service** | `internal/domain/service/availability.go` | 56-237 | 6 `domain/repository/*` interfaces (passed as method args) | YES — pure mocks via interfaces |
| **Repo duplicate** | `internal/repository/bookings.go` | 382-603 | Raw `*sql.DB` queries inline | NO — requires `sqlmock` or real DB |

The repo duplicate was introduced during Phase 1 (before the domain service existed) and was scheduled for deletion in Phase 3 (design.md line 215: `Remove CheckAvailability`). It still exists because Phase 3 hasn't been completed.

### 2.3 Gap Analysis: What the Repo DOESN'T Validate

For `CreateBooking` and `RescheduleBooking`, the current repo (`internal/repository/bookings.go`) performs ONLY the atomic overlap check via `WHERE NOT EXISTS`. It does NOT check:

- [ ] **Past time**: `ErrCodeSlotInPast` — never returned for Create/Reschedule
- [ ] **Business hours**: `ErrCodeBusinessClosed` — never returned for Create/Reschedule
- [ ] **Professional schedule**: `ErrCodeProfessionalNotWorking` — never returned for Create/Reschedule
- [ ] **Slot duration** within combined business+professional hours: `ErrCodeSlotOutOfHours` — never returned for Create/Reschedule
- [ ] **Service active**: `ErrCodeServiceNotActive` — partially checked by `CreateBookingUseCase` (line 69-71)
- [ ] **Professional active**: `ErrCodeProfessionalNotActive` — not checked at all

This means a booking created for a past time or outside business hours would pass through the use case undetected. The only guard is the atomic overlap check — a defense-in-depth mechanism, not a business validation.

### 2.4 Error Code Mapping Inconsistency

| Source | Error returned | Use case mapping | Resulting code |
|---|---|---|---|
| `AvailabilityService` | `*domain.SemanticError{Code: ErrCodeSlotInPast}` | Propagates as-is | `ErrCodeSlotInPast` |
| `AvailabilityService` | `*domain.SemanticError{Code: ErrCodeBusinessClosed}` | Propagates as-is | `ErrCodeBusinessClosed` |
| `BookingsRepo.Create` | `domain.ErrConflict` | → `ErrCodeBookingOverlap` (create_booking.go:89) | `ErrCodeBookingOverlap` |
| `BookingsRepo.Reschedule` | `domain.ErrConflict` | → `ErrCodeBookingOverlap` (reschedule_booking.go:58) | `ErrCodeBookingOverlap` |

The repo → use case error mapping (`ErrConflict` → `ErrCodeBookingOverlap`) is fragile: if the repo ever returned `ErrConflict` for a non-overlap reason, the use case would mislabel it.

## 3. Options Analysis

### Option A: Keep Current Pattern (0 changes)

| Dimension | Assessment |
|---|---|
| **Pros** | Zero effort. No regression risk. Current tests pass. |
| **Cons** | Inconsistent: `CheckAvailability` validates 5 steps; `CreateBooking`/`RescheduleBooking` validate 1 (overlap only). Two use cases can create bookings in the past or outside business hours. New use cases must re-derive which validator to call. |
| **Effort** | 0 |
| **Risk** | Medium — silent data quality degradation (past/out-of-hours bookings possible) |
| **Architecture fit** | Violates hexagonal architecture: two use cases bypass domain validation |

### Option B: Use Cases Call AvailabilityService Before Repo

`CreateBookingUseCase` and `RescheduleBookingUseCase` call `AvailabilityService.CheckAvailability` before dispatching to the repo.

| Dimension | Assessment |
|---|---|
| **Pros** | Consistent with `CheckAvailabilityUseCase`. Minimum new code. Domain service reused. |
| **Cons** | `AvailabilityService` isn't designed for this use case — it requires `AvailabilityDeps` with 6 repos, only 1 of which (`BookingsRepo.FindOverlapping`) is needed for validation during Create/Reschedule. The use cases already resolve service/professional data (e.g., `create_booking.go:62-71`), creating redundant resolution. Two calls: `CheckAvailability` (read preview) + repo `Create` (atomic write) — overlap could slip between them. |
| **Effort** | 2-3 hours |
| **Risk** | Medium — TOCTOU race: overlap check and insert are not atomic |
| **Architecture fit** | Partial: uses domain service but introduces 2-phase commit without atomics |

### Option C: New BookingValidator Domain Service (Recommended)

New `BookingValidator` in `internal/domain/service/` that owns the 5-step chain. All three use cases call it. The repo retains atomic overlap check as defense-in-depth but stops being the primary validator.

| Dimension | Assessment |
|---|---|
| **Pros** | Single source of truth for datetime validation. Testable in isolation with plain mocks (no DB). Consistent hexagonal architecture: domain service owns business rules; repo owns persistence. Extensible: new use cases call `validator.Validate(params, deps)`. Aligns with D5 (stateless domain services) and C1 (domain service contract). |
| **Cons** | New file/service to design (~180 LOC). Must coordinate with in-flight refactor P3.2. Repo must be refactored first (see §6). |
| **Effort** | 4-8 hours (design + impl + tests + repo cleanup) |
| **Risk** | Medium — dep on P3.2; PR budget: 3 use cases × 20 LOC + 1 service ~180 LOC + tests ~300 LOC ≈ 500 LOC (under 600-line budget) |
| **Architecture fit** | Excellent — aligns with D5, D9, C1; extracts business logic from repo per design.md P3 |

## 4. Recommended Option: C (BookingValidator)

### Rationale

1. **Single source of truth**: The 5-step chain currently exists in two places (`AvailabilityService` and `BookingsRepo.CheckAvailability`). Option C consolidates into one place and extends it to all datetime-validating use cases.

2. **Hexagonal architecture alignment**: Per design D5, domain services must be stateless and receive repository interfaces as arguments — `BookingValidator` follows this exactly. Per C1 (domain service contract): "Business logic that spans multiple entities OR requires repository calls belongs in a domain service." The 5-step chain spans 6 repos — it's the textbook definition of when to use a domain service.

3. **Testability**: Currently, datetime validation embedded in the repo (`BookingsRepo.CheckAvailability`, `BookingsRepo.RescheduleBooking`) requires `sqlmock` to test. A pure domain service can be tested with interface mocks — 10× faster, no SQL assertion fragility.

4. **Extensibility**: Future use cases (`BulkCreateBookingsUseCase`, `MassRescheduleUseCase`) call `validator.Validate(...)` once. No decision paralysis about which validator to call.

5. **Repo cleanup**: The repo's `CheckAvailability` (221 LOC duplicate) is deleted. The repo's `CreateBooking`/`RescheduleBooking` retain ONLY the atomic overlap check (defense-in-depth, per Decisión 11). The repo stops being a validator and becomes purely a persistence layer.

### What Changes

| File | Action | LOC impact |
|---|---|---|
| `internal/domain/service/booking_validator.go` | NEW — 5-step chain as `BookingValidator.Validate()` | +180 |
| `internal/domain/service/booking_validator_test.go` | NEW — table-driven tests for each step | +300 |
| `internal/domain/service/availability.go` | MODIFY — `CheckAvailability` delegates to `BookingValidator.Validate` (or becomes a thin wrapper that also resolves service/professional active status) | ~50 changed |
| `internal/application/usecase/create_booking.go` | MODIFY — inject `BookingValidator`, call `Validate` before `Create` | +25 |
| `internal/application/usecase/reschedule_booking.go` | MODIFY — inject `BookingValidator`, call `Validate` before `Reschedule` | +25 |
| `internal/application/usecase/check_availability.go` | MODIFY — inject `BookingValidator` instead of `AvailabilityChecker` | ~15 changed |
| `internal/application/usecase/*_test.go` | MODIFY — add validation scenarios | +200 |
| `internal/repository/bookings.go` | MODIFY — remove `CheckAvailability` method; keep atomic overlap in `CreateBooking`/`RescheduleBooking` | -250 |

**Total**: ~500 LOC new/changed. Under the 600-line review budget.

## 5. Architectural Fit

### 5.1 Compose with `refactor-clean-architecture` (in-flight)

The `refactor-clean-architecture` change is at P3.2 ("next" — repos implementing `domain.*Repository`). The `BookingValidator` depends on the post-P3 state:

- **NOW (pre-P3)**: `BookingsRepo` has `CreateBooking`, `RescheduleBooking`, `GetBooking` — does NOT implement `domain/repository/BookingsRepo` (which has `Create`, `Reschedule`, `FindByID`).
- **AFTER P3.2**: `BookingsRepo` implements `domain/repository/BookingsRepo` with clean method signatures `Create(ctx, *entity.Booking)`, `Reschedule(ctx, id, start, end time.Time)`.

**The `BookingValidator` MUST be developed against the post-P3 interface.** This means:
- Either wait for P3.2 to merge, then start `feat-booking-validator-service`.
- Or start development on a branch based on the P3.2 branch (chained PR).
- The validator's `BookingValidatorDeps` struct will reference `domain/repository/BookingsRepo` — which doesn't have a concrete implementation yet.

### 5.2 Where Does BookingValidator Live?

```
internal/domain/service/
├── availability.go      # AvailabilityChecker interface + AvailabilityService
├── booking_validator.go # BookingValidator (NEW)
└── mocks_test.go        # Shared test mocks
```

`BookingValidator` belongs in `internal/domain/service/` alongside `availability.go` because:
- It satisfies D5 (stateless, no DB access, receives repo interfaces as arguments).
- It operates on domain entities and repository interfaces — pure domain layer.
- It is NOT an application concern: use cases don't own validation rules; they orchestrate them.

### 5.3 Deeper Hexagonal Implication

The current `AvailabilityService` performs BOTH entity resolution (service/professional/business-profile lookup and active-status check) AND datetime validation (the 5-step chain). After introducing `BookingValidator`, these concerns can be separated:

- **`BookingValidator`**: pure datetime validation. Receives already-resolved data. No entity lookups.
- **`AvailabilityService`** (or use case): entity resolution + active-status check, then delegates to `BookingValidator`.

This split is consistent with Single Responsibility: the validator answers "is this time slot valid?", not "is the service active?".

## 6. Open Questions

### Q1: Does the repo stop validating entirely?
**Current**: `BookingsRepo.CreateBooking` runs an atomic `INSERT ... WHERE NOT EXISTS` (overlap check).
**Under C**: The validator checks overlap before the use case calls the repo. The repo's `WHERE NOT EXISTS` becomes defense-in-depth: it catches TOCTOU races between the validator's read and the repo's write.

**Recommendation**: Keep the atomic overlap check in the repo as defense-in-depth. The repo should NOT retry or return a business error on overlap — it should return a technical error (e.g., `domain.ErrConflict`) that the use case maps to `ErrCodeBookingOverlap` as a fallback. This is already the current pattern (issue #22, #23 confirm).

### Q2: What's the new repo contract for Create/Reschedule?
**Current domain interface** (`internal/domain/repository/bookings.go`):
```go
Create(ctx context.Context, b *entity.Booking) error
Reschedule(ctx context.Context, id string, newStart, newEnd time.Time) error
```

**Post-validator**: These signatures stay. The repo's job is persistence, not validation. The use case calls validator first, then repo. The repo's atomic overlap check is a safety net, not the primary validator.

### Q3: How do error codes unify?
**Current**: `AvailabilityService` returns direct `*domain.SemanticError`; use cases map `ErrConflict` → `ErrCodeBookingOverlap`.
**Under C**: `BookingValidator.Validate` returns `*domain.SemanticError` with the correct code directly. The use case propagates as-is. The repo's error mapping (`ErrConflict` → `ErrCodeBookingOverlap`) becomes a fallback, not the primary path.

No new error codes needed. The existing `ErrCodeXxx` taxonomy covers all cases: `BUSINESS_CLOSED`, `PROFESSIONAL_NOT_WORKING`, `SLOT_OUT_OF_HOURS`, `BOOKING_OVERLAP`, `SLOT_IN_PAST`, `SERVICE_NOT_ACTIVE`, `PROFESSIONAL_NOT_ACTIVE`.

### Q4: Should BookingValidator also check service/professional active status?
**Current**: `AvailabilityService` checks both (lines 67-72, 78-83). `CreateBookingUseCase` checks service active (line 69-71) but not professional active.
**Recommendation**: Entity resolution and active-status checks belong in the use case (they're part of business flow orchestration), not in the validator. The validator should receive already-resolved entities and focus on datetime rules. This keeps the validator pure and testable with minimal deps.

### Q5: Sequencing with refactor-clean-architecture P3.2
The `BookingValidator` depends on the post-P3 interface. Option:
- **A (wait)**: Develop after P3.2 merges. Low risk, serializes work.
- **B (branch chain)**: Develop on a branch based on P3.2. Parallel work, higher coordination.

**Recommendation**: Option A (wait). P3.2 is the next item; let it complete first. The validator change is small (500 LOC) and independent — it won't block anything.

## 7. Test Coverage Plan

### Current gaps

| Use case | Validated scenarios | Missing scenarios |
|---|---|---|
| `CheckAvailability` | Auth, DTO mapping, checker delegation | (covered by availability service tests) |
| `CreateBooking` | Auth, empty fields, service lookup, overlap error mapping | **Past time**, **business closed**, **professional not working**, **slot out of hours**, **professional not active** |
| `RescheduleBooking` | Auth, not found, cross-tenant, cancelled status, zero time, overlap error mapping | **Past time**, **business closed**, **professional not working**, **slot out of hours** |

### Post-validator coverage

Under TDD (strict_tdd: true), tests for each validation step are written BEFORE the validator implementation:

1. `BookingValidator` unit tests (table-driven, mock deps):
   - [ ] Valid slot → returns nil
   - [ ] Past time → `ErrCodeSlotInPast`
   - [ ] Business closed (exception) → `ErrCodeBusinessClosed`
   - [ ] Business closed (JSON fallback) → `ErrCodeBusinessClosed`
   - [ ] Professional not working → `ErrCodeProfessionalNotWorking`
   - [ ] Slot ends after close → `ErrCodeSlotOutOfHours`
   - [ ] Slot starts before business open → `ErrCodeSlotOutOfHours`
   - [ ] Slot starts before professional start → `ErrCodeSlotOutOfHours`
   - [ ] Overlap → `ErrCodeBookingOverlap`

2. Use case tests for `CreateBooking` and `RescheduleBooking`:
   - [ ] Past time → `ErrCodeSlotInPast` (was: no test)
   - [ ] Overlap → `ErrCodeBookingOverlap` (already tested via mock)

## 8. References

### GitHub Issues
- [#22](https://github.com/egkike/mcp-appointments-crm/issues/22) — RescheduleBookingUseCase delegates datetime validation to the repo
- [#23](https://github.com/egkike/mcp-appointments-crm/issues/23) — Datetime validation is scattered across 3 places

### Design Documents
- `openspec/changes/refactor-clean-architecture/design.md` — D5 (stateless domain services), D9 (manual DI), P3 (repo refactor)
- `openspec/changes/refactor-clean-architecture/specs/architecture/spec.md` — C1 (domain service contract), C2 (use case contract)

### Key Files
| File | LOC | Role |
|---|---|---|
| `internal/domain/service/availability.go` | 237 | Domain service: 5-step validation chain (seed for validator) |
| `internal/domain/repository/bookings.go` | 46 | Domain interface: `BookingsRepo` contract |
| `internal/domain/errors.go` | 74 | Error codes: `ErrCodeXxx` taxonomy |
| `internal/repository/bookings.go` | 620 | Repo: duplicate `CheckAvailability` (to delete), `CreateBooking`, `RescheduleBooking` |
| `internal/application/usecase/create_booking.go` | 95 | Use case: no datetime validation |
| `internal/application/usecase/reschedule_booking.go` | 72 | Use case: no datetime validation |
| `internal/application/usecase/check_availability.go` | 53 | Use case: delegates to `AvailabilityChecker` |
| `internal/application/usecase/reschedule_booking_test.go` | 315 | Tests: auth + dispatch + error mapping (no validation) |
| `internal/application/usecase/create_booking_test.go` | 336 | Tests: auth + empty fields + error mapping (no validation) |
| `internal/application/usecase/check_availability_test.go` | 272 | Tests: auth + DTO mapping + checker delegation |

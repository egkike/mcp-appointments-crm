# Proposal: feat-booking-validator-service

> **Change**: feat-booking-validator-service
> **Date**: 2026-08-03
> **Status**: Accepted (Q-DRIFT-1 resolved)
> **Related**: Issues #22, #23
> **Depends on**: refactor-clean-architecture P3.3a (merged as #36)

## Why

Today, booking datetime validation is scattered across three places: the `AvailabilityService` domain service (5-step chain, used only by `CheckAvailabilityUseCase`), the SQL `BookingsRepo` atomic overlap check (the only check reached by `CreateBooking`/`RescheduleBooking` use cases), and an entity-resolution step inlined in each use case. The net effect is that **two of the three booking use cases validate only overlap** — a booking in the past or outside business hours passes through the use case undetected. The repo's atomic `WHERE NOT EXISTS` is a defense-in-depth concurrency guard, not a business validator.

This creates three concrete problems. (1) **ErrCode gap**: `Create`/`Reschedule` never emit `ErrCodeSlotInPast`, `ErrCodeBusinessClosed`, `ErrCodeProfessionalNotWorking`, `ErrCodeSlotOutOfHours`, or `ErrCodeProfessionalNotActive`. (2) **Knowledge location**: business rules live behind a `*sql.DB`, forcing use case tests to mock SQL instead of behaviour. (3) **Fragile error mapping**: the use cases translate repo `domain.ErrConflict` → `ErrCodeBookingOverlap` on the assumption that any conflict returned by the repo is an overlap — an assumption that holds today but cannot be enforced by the type system.

Left as-is, every new booking use case (`BulkCreateBookings`, `MassReschedule`) re-derives which validator to call, and the silent data-quality gap widens.

## What Changes

Surface area. Not code.

- [ ] NEW: `internal/domain/service/booking_validator.go` (~120 LOC): `BookingValidator` domain service that **delegates the 5-step chain to the shared helper** (see below) and owns the validate-or-skip orchestration. Receives already-resolved entities.
- [ ] NEW: `internal/domain/service/booking_time_validator.go` (~140 LOC): **shared helper** `ValidateBookingTimeSlot` extracted from the 5-step chain. Pure function, no I/O, no state. Used by both `AvailabilityService.CheckAvailability` and `BookingValidator.Validate` (per resolved Q-DRIFT-1).
- [ ] NEW: `internal/domain/service/booking_validator_test.go` (~250 LOC): full TDD coverage, table-driven, hand-rolled interface mocks (reusing the `mocks_test.go` pattern from P1.4d). Includes a regression test asserting `BookingValidator.Validate` and `AvailabilityService.CheckAvailability` produce identical error codes for the same input.
- [ ] NEW: `internal/domain/service/booking_time_validator_test.go` (~150 LOC): exhaustive matrix of past / business hours / professional schedule / slot-within-hours / overlap.
- [ ] MODIFIED: `internal/domain/service/availability.go` (~40 LOC refactor): the inline 5-step chain is REPLACED by a call to the shared helper. All 16 existing `availability_test.go` subtests must continue to pass without modification (regression gate).
- [ ] MODIFIED: `internal/application/usecase/create_booking.go` (~30 LOC change): inject `BookingValidator`, resolve service/professional/business-profile, call `Validate` before `bookings.Create`. Service-active check (line 69-71) stays in the use case (entity-resolution concern, per exploration Q4 decision).
- [ ] MODIFIED: `internal/application/usecase/create_booking_test.go` (~50 LOC new tests): validation happy path + 7 error codes as table-driven subtests; add `mockBookingValidator`.
- [ ] MODIFIED: `internal/application/usecase/reschedule_booking.go` (~30 LOC change): inject `BookingValidator`, call `Validate` before `bookings.Reschedule`.
- [ ] MODIFIED: `internal/application/usecase/reschedule_booking_test.go` (~50 LOC new tests): same matrix.
- [ ] UNCHANGED: `internal/application/usecase/check_availability.go` — stays on `AvailabilityService` directly (decision 2). The wire shape is unchanged; only `AvailabilityService` internals move.
- [ ] UNCHANGED: `internal/repository/bookings.go` — atomic overlap check in `Create` (line 85) and `Reschedule` (line 239) stays as defense-in-depth per Decisión 11.

## Architectural Decisions

1. **Opción C — `BookingValidator` domain service** in `internal/domain/service/booking_validator.go`. Rationale: single source of truth for datetime rules; pure domain layer (stateless, receives repo interfaces as args per D5); testable without `sqlmock` (10× faster, no SQL-assertion fragility); aligns with C1 (domain service contract — business logic spanning multiple repos belongs in a service). Repo retains atomic overlap check as defense-in-depth (Decisión 11) to catch TOCTOU races between the validator's read and the repo's write.
2. **`CheckAvailabilityUseCase` stays on `AvailabilityService`.** Only `CreateBookingUseCase` and `RescheduleBookingUseCase` route through `BookingValidator`. Rationale: `CheckAvailability` is a non-authoritative preview that already works and is fully tested in isolation; migrating it is a behavioural no-op that would expand blast radius for no contract change. The 5-step chain is therefore **extracted** (not **moved**) — `AvailabilityService` keeps its copy, `BookingValidator` gets its own, and a follow-up change may collapse them via a shared helper.
3. **3 incremental PRs.** PR #A — service + tests; PR #B — CreateBooking integration + tests; PR #C — RescheduleBooking integration + tests + cleanup. Rationale: each PR independently passes `go test -race ./...`, each is under the 600-line review budget (config), and the risk-relevant use-case wiring lands in isolation so a regression blames one use case.
4. **Keep the fragile `domain.ErrConflict` → `ErrCodeBookingOverlap` mapping** in the use cases. The validator runs BEFORE the repo, so the only remaining path to `ErrConflict` is the repo's atomic overlap guard — the mapping is therefore correct-by-construction even if not provably so. Cleanup of this fragility is deferred to a follow-up (documented as Out of Scope).
5. **Q-DRIFT-1 RESOLVED: shared helper from PR #A.** The 5-step chain is EXTRACTED into `internal/domain/service/booking_time_validator.go` as a pure helper `ValidateBookingTimeSlot(ctx, slot, deps) *domain.SemanticError`. Both `AvailabilityService.CheckAvailability` (refactored) and `BookingValidator.Validate` (new) call it. Rationale: single source of truth from day 1; eliminates the drift risk that decision 2 implicitly accepted; the helper is pure (no I/O, no state) so its extraction carries zero risk of breaking `AvailabilityService`'s 16-subtest regression gate. Cost: PR #A grows from ~370 to ~560 LOC, and includes a refactor of `AvailabilityService` (the only file we agreed to leave UNCHANGED before this resolution). The refactor is mechanical (extract-method) and the regression gate is strict.

## Scope Boundaries

**In scope** (this change):
- `BookingValidator` domain service
- `ValidateBookingTimeSlot` shared helper (extracted from the 5-step chain, per resolved Q-DRIFT-1)
- Mechanical refactor of `AvailabilityService.CheckAvailability` to call the helper (with strict 16-subtest regression gate)
- 7 error codes (preserved as-is — no renames, no new codes)
- `CreateBookingUseCase` + `RescheduleBookingUseCase` integration
- 3 incremental PRs (PR #A grows because of the shared helper + the `AvailabilityService` refactor)

**Out of scope** (explicit non-goals):
- `CheckAvailabilityUseCase` migration to `BookingValidator` (stays on `AvailabilityService` per decision 2)
- Error code renames or consolidation
- Removal of the fragile `ErrConflict` → `ErrCodeBookingOverlap` mapping (deferred per decision 4)
- FTS5-based validation performance optimization
- Telemetry/observability
- `BulkCreateBookings` or `BulkRescheduleBookings` use cases (mentioned in #22 but not in this scope)
- Moving service/professional active-status checks into the validator (stays in the use case per exploration Q4)

## Capabilities

> Contract for the specs phase. Research performed against `openspec/specs/`.

### New Capabilities
- `booking-validator`: the `BookingValidator` domain service — calls `ValidateBookingTimeSlot` (the shared helper) and returns the appropriate `*domain.SemanticError` for any invalid datetime. Used by `CreateBookingUseCase` and `RescheduleBookingUseCase` before repo dispatch.
- `booking-time-validator`: the shared `ValidateBookingTimeSlot` helper — pure function with the 5-step chain (business hours, professional schedule, slot within hours, overlap, past). Used by both `BookingValidator` and `AvailabilityService` (Q-DRIFT-1 resolution).

### Modified Capabilities
- `bookings`: the existing `bookings` spec mandates the 5-step chain only for `CheckAvailability` and the atomic overlap guard for `Create`/`Reschedule`. This change adds the requirement that `Create` and `Reschedule` MUST run the full 5-step validation before dispatching to the repo (closing the ErrCode gap). The atomic overlap check remains as defense-in-depth.
- `availability`: the existing `availability` spec (`openspec/specs/availability/spec.md`) is preserved — the public contract of `AvailabilityService.CheckAvailability` does not change. Internally, the 5-step chain is replaced by a call to `ValidateBookingTimeSlot`. All 16 existing subtests pass unmodified (regression gate).

## Rollout

3 PRs (per decision 3):

1. **PR #A — `BookingValidator` + shared helper + `AvailabilityService` refactor**: `internal/domain/service/booking_validator.go` + `booking_validator_test.go` + `booking_time_validator.go` + `booking_time_validator_test.go` + mechanical refactor of `availability.go` to call the shared helper. ~560 LOC. Independently mergeable; no use case callers yet. **Regression gate**: all 16 existing `availability_test.go` subtests MUST pass unmodified.
2. **PR #B — CreateBooking integration**: wire `BookingValidator` into `CreateBookingUseCase`, add 7 table-driven validation subtests. ~80 LOC change.
3. **PR #C — RescheduleBooking integration + cleanup**: wire into `RescheduleBookingUseCase`, add 7 subtests, final cleanup. ~80 LOC change.

Each PR MUST pass `go test -v -race ./...` independently (strict TDD).

## Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Repository atomic overlap check becomes unreachable if the validator always catches overlap first → defense-in-depth silently dead | Low | Decisión 11 keeps the repo guard explicitly; PR #B/#C tests assert the repo guard still fires on a TOCTOU race (inject a validator-then-conflict sequence in the use case test). |
| 7 error codes × 2 use cases = 14 new subtests, wide test surface | Medium | Table-driven subtests share a single mock setup; matrix encoded as `[]struct{ name; input; wantCode }`. |
| Drift between `AvailabilityService.CheckAvailability` and `BookingValidator.Validate` if the 5-step chain is implemented twice | RESOLVED | Q-DRIFT-1 resolved: shared helper `ValidateBookingTimeSlot` in PR #A eliminates this risk by giving both services a single caller. |
| 2 new mock interfaces needed (`BookingValidator` for use cases; existing `mockBookingsRepo` extended) | Low | Reuse existing `mockBookingsRepo` from use case tests; add `mockBookingValidator` following the same function-table pattern. |
| Exploration drift: exploration was written pre-P3.2/P3.3a and assumed the repo still had a 244-LOC `CheckAvailability` duplicate and that `BookingsRepo` had pre-P3 method names | Low | Verified against current `main`: P3.3a removed the duplicate (`11f237f`), `domain.BookingsRepo` has 10 clean methods, use cases already call `Create`/`Reschedule`/`FindByID`. Proposal surface area reflects the post-P3.3a state. The exploration's "remove repo duplicate" line item is therefore already DONE and is NOT part of this change. |

## Open Questions

- **Q-DRIFT-1 RESOLVED**: shared helper from PR #A (user decision). The 5-step chain is extracted into `internal/domain/service/booking_time_validator.go` as a pure helper `ValidateBookingTimeSlot(ctx, slot, deps) *domain.SemanticError`. Both `AvailabilityService.CheckAvailability` (refactored) and `BookingValidator.Validate` (new) call it. See decision 5 in Architectural Decisions.

No other open questions — decisions 1–5 were resolved by the user.

## Rollback Plan

Each PR is independently revertible via `git revert <merge>`:

- **PR #A revert**: delete `internal/domain/service/booking_validator.go` + `_test.go` + `booking_time_validator.go` + `_test.go`, and revert the `availability.go` refactor (re-inline the 5-step chain). No production callers of `BookingValidator` exist at PR #A merge time; reverting the helper extraction restores the pre-refactor `AvailabilityService` byte-identical to `11f237f`. Revert is clean.
- **PR #B revert**: remove `BookingValidator` injection from `CreateBookingUseCase`, remove the 7 new subtests. The use case returns to its current repo-delegation behaviour.
- **PR #C revert**: same as #B for `RescheduleBookingUseCase`.

No DB migration, no schema change, no auth change → no data rollback required. Restoring `main` to commit `11f237f` is the worst-case rollback.

## Dependencies

- `refactor-clean-architecture` P3.2a (PR #33, merged) — clean `BookingsRepo` interface ✓
- `refactor-clean-architecture` P3.3a (PR #36, merged) — repo `CheckAvailability` duplicate removed, `AvailabilityService` is sole source of the 5-step chain ✓
- This change unblocks `refactor-clean-architecture` P3.3b-d (move business logic out of repos) — once `BookingValidator` owns datetime rules, P3.3c's `validateService`/`validateProfessional` items can be addressed without touching repo code.

## Success Criteria

- [ ] `BookingValidator.Validate` returns the correct `*domain.SemanticError` code for all 7 invalid scenarios + `nil` for the happy path (table-driven, hand-rolled mocks, no `sqlmock`).
- [ ] `CreateBookingUseCase` and `RescheduleBookingUseCase` emit every one of the 7 `ErrCode*` codes — closing the ErrCode gap documented in exploration §2.3.
- [ ] `go test -v -race ./...` passes after each of the 3 PRs.
- [ ] No new `ErrCode*` constant introduced; existing taxonomy unchanged.
- [ ] `internal/repository/bookings.go` is NOT modified (atomic overlap check stays as defense-in-depth).
- [ ] `CheckAvailabilityUseCase` remains byte-identical to `11f237f`. `AvailabilityService` IS REFACTORED (5-step chain extracted to shared helper) but BEHAVIOUR is byte-equivalent: signature, return type, error codes, and all 16 existing `availability_test.go` subtests pass unmodified. The byte-equivalence is asserted by the regression gate (REQ-AV-2).
- [ ] No `database/sql` import added to `internal/domain/` (zero-dependency rule preserved).

## References

- `openspec/changes/feat-booking-validator-service/exploration.md` (analysis, 249 LOC)
- `openspec/changes/refactor-clean-architecture/tasks.md` (P3.3a `[x]` closed; P3.3b-d, P3.5, P4 still pending → unblocked by this change)
- `openspec/specs/bookings/spec.md` (existing capability — Modified per above)
- `internal/domain/service/availability.go` (5-step chain seed — MODIFIED in PR #A via mechanical extract-method; BEHAVIOUR byte-equivalent, 16-subtest regression gate)
- `internal/domain/errors.go` (`SemanticError` + 7 `ErrCode*` constants — UNCHANGED)
- `internal/domain/repository/bookings.go` (10-method `BookingsRepo` interface — UNCHANGED)
- `internal/repository/bookings.go` (atomic overlap in `Create`/`Reschedule` — UNCHANGED per Decisión 11)
- Issues #22, #23
- PRs #33, #34, #35, #36 (refactor-clean-architecture P3.2 + P3.3a)
# Tasks: feat-booking-validator-service

> **Change**: feat-booking-validator-service
> **Status**: Planned
> **Related**: Issues #22, #23
> **PRs**: 3 (incremental rollout, see `proposal.md` §Rollout)

## PR #A — BookingValidator + shared helper + AvailabilityService refactor

> **Branch**: `feat/feat-booking-validator-apply-pr-a`
> **Estimated LOC**: ~560
> **Pre-flight**: go fmt/vet/build/lint/test -race all pass
> **GGA**: required
> **Judgment Day**: required (2 judges)
> **Regression gate**: 16 existing `availability_test.go` subtests MUST pass unmodified

### RED phase (write tests first)

- [x] TASK-A.1 — Write `internal/domain/service/booking_time_validator_test.go` with table-driven tests for the 9 subtests per design.md §4.1:
  > Actual impl: `TestValidateBookingTimeSlot` — 9 table-driven subtests (past_time, business_closed_exception, business_closed_json_fallback, professional_not_working, slot_ends_after_close, slot_starts_before_business_open, slot_starts_before_professional_start, overlap_detected, all_pass) + overlap-call short-circuit assertions via a call-count mock.
  1. `past_time` — `Start < time.Now()` → `ErrCodeSlotInPast`, no overlap query
  2. `business_closed_exception` — closed-day exception → `ErrCodeBusinessClosed`
  3. `business_closed_json_fallback` — empty weekly JSON → `ErrCodeBusinessClosed`
  4. `professional_not_working` — no schedule row → `ErrCodeProfessionalNotWorking`
  5. `slot_ends_after_close` — `slotEnd > effectiveClose` → `ErrCodeSlotOutOfHours`
  6. `slot_starts_before_business_open` — `slotStart < businessOpen` → `ErrCodeSlotOutOfHours`
  7. `slot_starts_before_professional_start` — `slotStart < proStart` → `ErrCodeSlotOutOfHours`
  8. `overlap_detected` — mocked `FindOverlapping` returns 1 → `ErrCodeBookingOverlap`
  9. `all_pass` — valid slot → `nil`
  > Test table shape: `[]struct{ name string; input SlotInput; deps BookingTimeValidatorDeps; wantCode *string }`. Tests MUST fail (function doesn't exist yet).
- [x] TASK-A.2 — Write `internal/domain/service/booking_validator_test.go` with table-driven tests (12 scenarios per the spec)
  > Actual impl: `TestBookingValidator_Validate` — 12 table-driven subtests driven through `Validate`: the 9-step matrix + inactive_service_not_validated, inactive_professional_not_validated, first_error_short_circuits (past AND overlap → SlotInPast, overlap mock never called).
- [x] TASK-A.3 — Verify pre-flight: `go test -v -race ./internal/domain/service/` shows expected RED (new tests fail with "undefined: ValidateBookingTimeSlot" / "undefined: BookingValidator").
  > Actual impl: RED confirmed — build failure `undefined: SlotInput`, `undefined: BookingTimeValidatorDeps`, `undefined: ValidateBookingInput`.

### GREEN phase (implement to pass)

- [x] TASK-A.4 — Implement `internal/domain/service/booking_time_validator.go` (~140 LOC)
  > Package-private function `ValidateBookingTimeSlot(ctx, slot, deps) *domain.SemanticError`. Pure function, no I/O, short-circuits on first error. Order: past → business hours → professional schedule → slot-within-hours → overlap.
  > Actual impl: implemented per design.md §3.2 / spec REQ-BTV-2. Defines `BookingOverlapReader`, `SlotInput`, `BookingTimeValidatorDeps`. Step 4 panics on nil/non-positive Service.Duration (REQ-BTV-2). 5-step order: past → business (exception-aware + weekly JSON) → professional schedule → within-combined-hours → overlap.
- [x] TASK-A.5 — Implement `internal/domain/service/booking_validator.go` (~120 LOC)
  > Stateless struct `BookingValidator` with method `Validate(ctx, input ValidateBookingInput) *domain.SemanticError`. Calls the helper.
  > Actual impl: `NewBookingValidator()` + `Validate` + `ValidateBookingInput` per design §3.1. Mechanical mapping to `SlotInput` (Professional.ID hoisted to ProfessionalID) then delegates to helper.
- [x] TASK-A.6 — Refactor `internal/domain/service/availability.go` (~40 LOC change)
  > Replace the inline 5-step chain in `CheckAvailability` with a call to `ValidateBookingTimeSlot(ctx, slot, deps)`. Method signature, return type, and error codes UNCHANGED. Behaviour byte-equivalent.
  > Actual impl: resolution block (svc/pro/profile/loc/startTime + exception + schedule lookups with non-ErrNotFound error wiring) stays inline; the 5-step chain replaced by a single delegate call. Signature/return type unchanged.
- [x] TASK-A.7 — Verify pre-flight: `go test -v -race ./...` passes
  > Specifically: 16 existing subtests in `availability_test.go` pass unmodified + new tests in `booking_time_validator_test.go` (9 subtests) and `booking_validator_test.go` (12 subtests) pass.
  > Actual impl: full `go test -v -race ./...` green — all 9 packages PASS, 0 FAIL, 0 races. Regression gate: 15 `TestCheckAvailability` subtests + `TestHHMMToMinutes` pass with ZERO diff to `availability_test.go`.

### REFACTOR phase (cleanup)

- [x] TASK-A.8 — Add the `BookingOverlapReader` interface declaration (per design R7 mitigation)
  > If R7 (import cycle) is observed, declare a zero-dep interface in `internal/domain/repository/booking_overlap_reader.go` and have both `BookingValidator` and `ValidateBookingTimeSlot` accept it. The concrete `mockBookingsRepo` already satisfies it structurally.
  > Actual impl: R7 NOT observed in PR #A — no use case imports `internal/domain/service` in this PR, so no cycle can form. `BookingOverlapReader` defined locally in `booking_time_validator.go` per design §3.2. `mockBookingsRepo` satisfies it structurally (compile-time proven: `BookingTimeValidatorDeps.Bookings` is assigned a `*mockBookingsRepo`).
- [x] TASK-A.9 — Run `gofmt -s -w` on the new files
  > Actual impl: `gofmt -s -w` on the 4 new files + `availability.go`; `gofmt -l` empty (all formatted).
- [x] TASK-A.10 — Update `openspec/changes/feat-booking-validator-service/tasks.md` to mark all TASK-A.* as [x] with one-line "Actual impl" notes
  > Actual impl: this file — tasks A.1–A.10 marked [x] with evidence notes.

### Commit + Push + PR

- [ ] TASK-A.11 — Create branch `feat/feat-booking-validator-apply-pr-a` from main
- [ ] TASK-A.12 — `git add` only the 4 new files + `availability.go` + `tasks.md`
  > Do NOT add the parked `openspec/changes/feat-booking-validator-service/exploration.md` (it's a record, not part of this PR)
- [ ] TASK-A.13 — GGA pre-commit. If GGA picks up the parked file, run `git rm --cached openspec/changes/feat-booking-validator-service/exploration.md && git commit --amend --no-edit`
- [ ] TASK-A.14 — Commit message: `feat(domain/service): PR-A BookingValidator + ValidateBookingTimeSlot helper + AvailabilityService refactor (#22, #23)`
  > Body: Adds the new BookingValidator domain service and the shared ValidateBookingTimeSlot helper, refactors AvailabilityService.CheckAvailability to delegate to the helper (byte-equivalent, 16-subtest regression gate). Closes the ErrCode gap for #22, #23.
- [ ] TASK-A.15 — Push to origin, open PR via `gh pr create --base main --head feat/feat-booking-validator-apply-pr-a --title "feat(domain/service): PR-A BookingValidator + shared helper + AvailabilityService refactor (#22, #23)"`
- [ ] TASK-A.16 — Judgment Day: 2 judges (jd-judge-a, jd-judge-b) in parallel, blind, against commit SHA
  > If both judges report zero severe findings → terminal_state=approved
  > If any severe finding → fix-actor round (max 2 rounds per skill contract)
- [ ] TASK-A.17 — After terminal_state=approved → ask user "¿Hacemos commit?" and wait for green light

### Pre-flight gates (verify before each commit)

- [ ] TASK-A.18 — `go fmt ./...` clean
- [ ] TASK-A.19 — `go vet ./...` clean
- [ ] TASK-A.20 — `go build -o /dev/null ./...` passes
- [ ] TASK-A.21 — `golangci-lint run ./...` 0 issues
- [ ] TASK-A.22 — `go test -v -race ./...` all pass (including the 16 availability_test.go subtests unmodified)

---

## PR #B — CreateBookingUseCase integration

> **Branch**: `feat/feat-booking-validator-apply-pr-b`
> **Estimated LOC**: ~80 change (3 files modified)
> **Pre-flight**: go fmt/vet/build/lint/test -race all pass
> **GGA**: required
> **Judgment Day**: required (2 judges)
> **Dependency**: PR #A merged

### Implementation

- [x] TASK-B.1 — Modify `internal/application/usecase/create_booking.go`: add `validator domain.BookingValidator` to the struct, add constructor param, inject call before `bookings.Create`
  > Actual impl: declared local `bookingValidator` interface (narrow contract, accept-interfaces-return-structs); extended `CreateBookingUseCase` struct with 5 new fields (validator, pros, bizProf, bizEx, schedules); extended `NewCreateBookingUseCase` constructor to 7 params.
- [x] TASK-B.2 — Resolve entities (service, professional, business profile) BEFORE calling validator
  > Actual impl: Resolve `pro` (Pros.FindByID), `profile` (BizProf.Get), timezone via `service.ParseBusinessTimezone`, `localStart` in business TZ, `dayOfWeek`, `exceptionDate` (BizEx.Get, ErrNotFound-tolerant), `schedule` (Schedules.FindByProfessionalAndDay, ErrNotFound-tolerant). Pattern matches AvailabilityService.CheckAvailability.
- [x] TASK-B.3 — On validator error, return as-is. On validator pass, dispatch to repo. On `domain.ErrConflict` from repo, map to `ErrCodeBookingOverlap` as today (TOCTOU)
  > Actual impl: `if semErr := uc.validator.Validate(...); semErr != nil { return nil, semErr }` (REQ-BK-10/11). `if errors.Is(err, domain.ErrConflict) { return ... ErrCodeBookingOverlap ... }` for TOCTOU (REQ-BK-12). Post-JD F1 (both judges): added `!pro.IsActive()` check after pro resolution, returning `ErrCodeProfessionalNotActive` (REQ-BV-4 failure modes; mirrors availability.go:78-83). Plus 9th test row `professional_not_active` in the matrix.
- [x] TASK-B.4 — Extend `internal/application/usecase/create_booking_test.go` with 8 table-driven subtests (matches design.md §4.2):
  1. `happy_path` — validator returns `nil`, repo returns `nil` → result.BookingID != ""
  2. `past_slot` — validator returns `ErrCodeSlotInPast` → use case returns same
  3. `business_closed` — validator returns `ErrCodeBusinessClosed` → use case returns same
  4. `professional_not_working` — validator returns `ErrCodeProfessionalNotWorking` → use case returns same
  5. `slot_out_of_hours` — validator returns `ErrCodeSlotOutOfHours` → use case returns same
  6. `overlap` — validator returns `ErrCodeBookingOverlap` → use case returns same
  7. `service_not_active` — use case's OWN active-status check (validator NOT called) → `ErrCodeServiceNotActive`
  8. `toctou_repo_overlap` — validator returns `nil`, repo returns `domain.ErrConflict` → use case maps to `ErrCodeBookingOverlap`
  9. `professional_not_active` — pro.IsActive() == false (post-JD F1) → use case returns `ErrCodeProfessionalNotActive`, validator NOT called
  > Subtests 2–6 prove the use case propagates validator errors unchanged (REQ-BK-10, REQ-BK-11). Subtests 7 and 9 prove the use case's pre-validator active check still works (REQ-BV-4 failure modes; the validator does NOT own active-status checks). Subtest 8 is the TOCTOU guard (REQ-BK-12) and proves the repo atomic check stays reachable.
  > Actual impl: 9 subtests pass in `TestCreateBookingUseCase_Execute` (table-driven). Pre-existing `TestCreateBookingUseCase` (8 auth/role/input subtests) also pass. The second happy path (client creates for themselves) was missing `bookRepo.CreateFn = ...; return nil` — fixed inline during completion.
- [x] TASK-B.5 — Add `mockBookingValidator` to test file (function-table pattern matching `internal/domain/service/mocks_test.go`)
  > Actual impl: `mockBookingValidator` with `OnValidate` field and panic-if-nil pattern. Plus stub methods for the 9 unused interface methods on the 4 new repo mocks (FindActive, Save, Update on ProfessionalsRepo; Update on BusinessProfileRepo; Create, List, Delete on BusinessHoursExceptionRepo; Upsert, Delete on SchedulesRepo) — each panics with a clear message to surface unexpected dependencies in tests.

### DI wiring (required in same PR — TASK-FU.3 superseded)

- [ ] TASK-B.6 — **MUST update** `cmd/mcp-server/main.go`: construct `NewBookingValidator()` once as a singleton, pass it to the new `NewCreateBookingUseCase(...)` constructor. The use case gains 5 new repo params (Pros, BizProf, BizEx, Schedules — see design.md §3.4) plus the validator; main.go MUST be updated in this PR or the build breaks. (TASK-FU.3 is partially superseded: PR #B handles CreateBooking wiring; PR #C handles RescheduleBooking wiring. Full P4 DI is the refactor-clean-architecture P4 task.)
  > **DEFERRED** to refactor-clean-architecture P4.1a. The `cmd/mcp-server/main.go` file does not exist yet — the project is `internal/`-only (library, no binary entry point). P4.1a is the planned task to create the main.go with full DI wiring (including the new validator + 5 repo params for both CreateBookingUseCase and RescheduleBookingUseCase). The build does not break in this PR because no caller of `NewCreateBookingUseCase` exists. PR #B completes the use case + tests; the wiring is the natural scope of P4.1a.

### Pre-flight + Commit + PR
- [x] TASK-B.7 — Create branch `feat/feat-booking-validator-apply-pr-b` from main
  > Actual impl: branch created from `main` HEAD `1aab45c` (post PR-A merge). Initial state had uncommitted modifications on main (rescued from sdd-apply sub-agent crash) — branch was created via `git checkout -b` to preserve the work.
- [ ] TASK-B.8 — `git add` modified files: `create_booking.go`, `create_booking_test.go`, `cmd/mcp-server/main.go`, `tasks.md`
  > Note: only 3 files actually modified (main.go does not exist). The list above is the original spec; reality is `create_booking.go`, `create_booking_test.go`, `mocks_test.go`, `tasks.md`. The parked `exploration.md` is excluded.
- [ ] TASK-B.9 — GGA pre-commit
- [ ] TASK-B.10 — Commit message: `feat(usecase): PR-B CreateBookingUseCase integrates BookingValidator (#22, #23)`
- [ ] TASK-B.11 — Push to origin and open PR via `gh pr create --base main --head feat/feat-booking-validator-apply-pr-b --title "feat(usecase): PR-B CreateBookingUseCase integrates BookingValidator (#22, #23)"`
- [ ] TASK-B.12 — Judgment Day: 2 judges in parallel
- [ ] TASK-B.13 — Ask user "¿Hacemos commit?" after approval
  > Note: orchestrator handles this gate (TASK-A.17 pattern from PR #A).
- [x] TASK-B.14 — `go test -v -race ./...` all pass
  > Actual impl: 10/10 packages PASS, 0 races. `TestCreateBookingUseCase` 8/8 + `TestCreateBookingUseCase_Execute` 8/8.
- [x] TASK-B.15 — `golangci-lint run ./...` 0 issues
  > Actual impl: 0 issues across all 8 enabled linters (sqlclosecheck, rowserrcheck, gosec, errorlint, gocritic, prealloc, bodyclose, noctx).

---

## PR #C — RescheduleBookingUseCase integration + cleanup

> **Branch**: `feat/feat-booking-validator-apply-pr-c`
> **Estimated LOC**: ~80 change
> **Dependency**: PR #B merged

### Implementation

- [ ] TASK-C.1 — Modify `internal/application/usecase/reschedule_booking.go`: same pattern as PR #B. The use case struct gains `validator domain.BookingValidator` plus the 5 new repo params (Pros, BizProf, BizEx, Schedules, plus the existing bookings). Constructor signature mirrors PR #B.
- [ ] TASK-C.2 — Load the existing booking first (`bookings.FindByID(ctx, input.BookingID)`) and run `CanReschedule` check as today. If `CanReschedule` fails, return the error. Only after `CanReschedule` passes does the use case resolve the new-slot entities (svc, pro, profile, schedule, exception) and call `validator.Validate(ctx, ...)`.
  > **Reschedule-specific difference vs Create**: the matrix runs after the existing-booking load + `CanReschedule` check. The subtests in TASK-C.4 are constructed to set up a valid pre-state (existing booking that passes `CanReschedule`) and then exercise the new-slot validation matrix.
- [ ] TASK-C.3 — On validator error, return as-is. On validator pass, dispatch to `bookings.Reschedule`. On `domain.ErrConflict` from repo, map to `ErrCodeBookingOverlap` as today (TOCTOU — same pattern as Create).
- [ ] TASK-C.4 — Extend `internal/application/usecase/reschedule_booking_test.go` with the same 8-row matrix as PR #B (design.md §4.3), adapted to reschedule input shapes (pre-load existing booking, then exercise validator + repo paths). Reuse `mockBookingValidator` from PR #B.
- [ ] TASK-C.5 — Final cleanup: verify no orphan code, all error codes emit correctly, run `go test -v -race ./...` end-to-end. The shared `BookingValidator` instance from `main.go` is now used by both `CreateBookingUseCase` and `RescheduleBookingUseCase` (single instance, two consumers).

### DI wiring (required in same PR — TASK-FU.3 partially superseded)

- [ ] TASK-C.6 — **MUST update** `cmd/mcp-server/main.go`: pass the existing `NewBookingValidator()` singleton (from PR #B) to the new `NewRescheduleBookingUseCase(...)` constructor. The 5 new repo params (Pros, BizProf, BizEx, Schedules, plus existing bookings) are added. main.go MUST be updated in this PR or the build breaks.

### Pre-flight + Commit + PR

- [ ] TASK-C.7 — Create branch `feat/feat-booking-validator-apply-pr-c` from main
- [ ] TASK-C.8 — `git add` modified files: `reschedule_booking.go`, `reschedule_booking_test.go`, `cmd/mcp-server/main.go`, `tasks.md`
- [ ] TASK-C.9 — GGA pre-commit
- [ ] TASK-C.10 — Commit message: `feat(usecase): PR-C RescheduleBookingUseCase integrates BookingValidator + cleanup (#22, #23)`
  > Body: Wires BookingValidator into RescheduleBookingUseCase. Both Create and Reschedule now emit the full ErrCode taxonomy. Closes issues #22 and #23.
- [ ] TASK-C.11 — Push to origin and open PR via `gh pr create --base main --head feat/feat-booking-validator-apply-pr-c --title "feat(usecase): PR-C RescheduleBookingUseCase integrates BookingValidator + cleanup (#22, #23)"`
- [ ] TASK-C.12 — Judgment Day: 2 judges in parallel
- [ ] TASK-C.13 — Ask user "¿Hacemos commit?" after approval
- [ ] TASK-C.14 — `go test -v -race ./...` all pass
- [ ] TASK-C.15 — `golangci-lint run ./...` 0 issues

---

## Out of scope (deferred follow-ups)

- [ ] TASK-FU.1 — Remove the fragile `domain.ErrConflict` → `ErrCodeBookingOverlap` mapping in the use cases (now provably safe but fragile by type system)
- [ ] TASK-FU.2 — CheckAvailabilityUseCase migration to BookingValidator (deferred per proposal decision 2)
- [ ] TASK-FU.3 — Full DI wiring cleanup in `cmd/mcp-server/main.go` (P4 of refactor-clean-architecture). Note: PR #B and PR #C of THIS change already do partial main.go wiring (passing the validator + new repo params to the new use case constructors). TASK-FU.3 is the P4 sweep that wires the remaining repos/handlers and removes any leftover no-op patterns.
- [ ] TASK-FU.4 — Error code renames or consolidation
- [ ] TASK-FU.5 — FTS5-based validation performance optimization
- [ ] TASK-FU.6 — Telemetry/observability for validation failures
- [ ] TASK-FU.7 — BulkCreateBookings / BulkRescheduleBookings use cases (mentioned in #22)

---

## Success Criteria

- [ ] PR #A merged: `BookingValidator` + `ValidateBookingTimeSlot` exist; `AvailabilityService` 16-subtest regression gate passes
- [ ] PR #B merged: `CreateBookingUseCase` emits all 7 ErrCode* values
- [ ] PR #C merged: `RescheduleBookingUseCase` emits all 7 ErrCode* values
- [ ] Issues #22 and #23 are closable (ErrCode gap closed, knowledge consolidated in domain service)
- [ ] `refactor-clean-architecture` P3.3b-d and P4 are unblocked

---

## Review Workload Forecast

- PR #A: 560 LOC (within 800 budget, no chained PR needed)
- PR #B: 80 LOC
- PR #C: 80 LOC
- Total: 720 LOC across 3 PRs
- Decision needed before apply: **No** (560 < 800 budget)
- Chained PRs recommended: **No**
- Chain strategy: **pending / single-PR-tracker**
- 400-line budget risk: **Low** (within approved 800 budget)

# Feature: micro-fixes — PR de micro-deuda agregado (backlog item 1)

**Status:** IN PROGRESS (started 2026-09-23 session)
**Branch:** `feat/micro-fixes` (code lane → feature branch + PR)
**Gate:** native review (Go code, default routing per AGENTS.md)
**Origin:** backlog obs 914 item (1); deferred findings R2-002 (review-67ce9afd81bae865,
telemetry main.go:425) + non-blocking follow-ups of odd/tasks/hermes-maintenance.md
(T1/T2/T3/T4 chains) + review informational backlog disposition.

## Scope

One PR aggregating three sources:
1. **R2-002 telemetry**: startup log counts derived from wiring, not hardcoded
   (main.go ~425: `"repos", 9, "usecases", 19` — drift-prone literals; counts
   correct since d7b65f7 but derived-by-hand).
2. **hermes-maintenance micro-debt** (agreed 6 items from feature doc follow-ups):
   - pluralization `"no abre los domingo."` → "los domingos" (validator copy,
     booking_time_validator.go:131/142 + spanishDayNames availability.go:51);
   - `parseBusinessHoursDayKey` Atoi accepts `"+1"/"01"/"007"` (business_profile.go:98);
   - `hhmmToMinutes` accepts unpadded `"9:00"` (datetime_helpers.go:38);
   - `deps.Bookings` not nil-guarded (availability.go:123, programmer-error surface);
   - `applyProfileUpdates` readability refactor (update_business_profile.go:106, R2-001);
   - sqlite_errors 1811 message-guard fragility for future triggers
     (sqlite_errors.go:94-99, R3-002/R4-002).
3. **Integration coverage** (T4 follow-ups): admin-denial cases, invalid-input
   integration cases, tool-count dedup between e2e_test.go and the suite.

## Tasks

- [x] **T1 — Domain validation micro-fixes**: plural day names used by validator messages
      (spanishDayNamesPlural: domingo→domingos, sábado→sábados, rest invariable; singular table
      kept + drift-guard test); strict day-key parse (exact "1".."7", rejects "+1"/"01"/"007"/"0"/"8");
      hhmmToMinutes strict padded HH:MM; HH:MM pattern deduplicated to single source of truth
      entity.ValidHHMM (service datetime_helpers + config/setup_loader migrated — third duplicate
      eliminated); integration assertion tightened to "no abre los domingos". Full pipeline green
      (fmt/vet/golangci 0 issues/build/test -race all packages). Commit pending owner approval.
- [ ] **T2 — deps.Bookings nil-guard**: fail-closed internal error when
      AvailabilityDeps.Bookings (→ BookingTimeValidatorDeps) is nil; test.
- [ ] **T3 — applyProfileUpdates refactor**: readability-only, behavior-preserving
      (R2-001); keep semantic identical partial-merge; tests must pass unchanged.
- [ ] **T4 — sqlite_errors message-guard hardening**: 1811 FK classification less
      brittle for future triggers (R3-002/R4-002); keep single-owner triggers
      unclassified; pin driver facts with tests.
- [ ] **T5 — Telemetry counts from wiring**: main.go startup log derives repos/usecases
      counts from the wired structures; test or compile-time assertion.
- [ ] **T6 — Integration coverage**: admin-denial (staff/admin RBAC matrix gap),
      invalid-input tool args through the mux, tool-count dedup (single source of
      truth for the 19-tools assertion).
- [ ] **T7 — Close-out**: full pre-flight pipeline (fmt/vet/golangci/build/test-race),
      native review gate (owner asked first), issue-first PR, owner squash-merge.

## Commits

(recorded per task on feature branch)

## Notes

- Watch: gentle-shell#1324/#924 (validator bug affects correction-heavy reviews;
  workaround = fresh START over corrected candidate).
- Never commit without asking; GGA runs on commit; no --no-verify.
- Review lifecycle owned by PARENT (skip review routing in delegated tasks).

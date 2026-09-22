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
      entity.ValidHHMM (service datetime_helpers + config/setup_loader + entity's own
      validateBusinessHoursJSON migrated — third duplicate eliminated); integration assertion
      tightened to "no abre los domingos". Full pipeline green; GGA PASSED on retry
      (first run hit STRICT_MODE ambiguous flake; 1 finding fixed: validateBusinessHoursJSON now
      calls ValidHHMM).
      Commit `cf2bbdc` on `feat/micro-fixes` — fix(domain): strict validation for day keys,
      HH:MM times and plural day messages.
- [x] **T2 — deps.Bookings nil-guard**: fail-closed internal error when BookingTimeValidatorDeps.Bookings
      is nil (step 5 dereferences it); contract docs corrected ("pure helper / no I/O" claim was false —
      steps 1-4 pure, step 5 single read). GGA surfaced a pre-existing CRITICAL fixed in the same
      work unit: midnight-crossing slots bypassed step 4 (slot end re-formatted from the wrapped wall
      clock → "01:00" = 60min passed any close bound); slot end now computed as absolute minute offset
      (start + duration), regression tests added (crossing rejected, same-day before close passes).
      GGA PASSED after 6 ambiguous retries (provider opencode-go/deepseek-v4.1-flash flaking on the
      STATUS line — 4+ consecutive). Commit `f6388bd` on `feat/micro-fixes` — fix(domain): fail closed
      on nil deps.Bookings and reject midnight-crossing slots.
- [x] **T3 — applyProfileUpdates refactor**: split into applyProfileScalarUpdates (6 deref-assign
      scalars) + applyProfileReferenceUpdates (13 verbatim pointer copies); composition entry unchanged;
      characterization table test pins the nil-keeps-value partial-merge over all 19 columns
      (the 13 pointer columns were previously untested at use-case level). Behavior-preserving;
      pipeline green (vet/golangci 0 issues/build/test -race). GGA hook failed on the parse-window
      flake across 6 retries; one fresh run returned full report STATUS: PASSED for the exact staged
      content (evidence: /tmp/gga-t3.log captured) — committed `0635b57` with --no-verify under
      explicit owner authorization, verdict recorded here. Commit `0635b57` on `feat/micro-fixes` —
      refactor(application): split applyProfileUpdates by assignment semantics.
      **Follow-up (upstream):** report gga parser window (30 lines hardcoded at line 1017 of gga
      2.10.1 script) vs agent-transcript providers — verdict lines land ~line 130 and get discarded
      as ambiguous; only PASSED results are cached, so retries re-roll the verdict each time.
- [x] **T4 — sqlite_errors message-guard hardening**: applicationTriggerMessages registry (single
      source of truth for RAISE(ABORT) markers sharing 1811, seeded with "single-owner invariant")
      + documented maintenance contract; 1811 branch now two-sided (FK message present AND no app
      marker; unknown 1811 stays unclassified, fail-safe direction preserved); isSingleOwnerViolation
      consults the registry; real-SQLite harness extended (dangling 787 / RESTRICT 1811 / trigger
      aborts / adversarial registered-marker-with-FK-string / fail-safe unknown message) + registry
      drift guard. GGA PASSED first attempt (compact response, STATUS at line 22). Commit on
      `feat/micro-fixes` — refactor(repository): two-sided guard for 1811 foreign-key classification.
- [x] **T5 — Telemetry counts from wiring**: startup log derives counts instead of hand-maintained
      literals (drift history: usecases 8→19, repos 6→11→9 — two fix commits needed). "usecases" ←
      new mcp.Server.ToolCount() (registry; 1:1 tool↔port↔use-case, key kept for ops grep); "repos" ←
      wiredRepoInventory() = len() over an auditable name list (single source of truth + maintenance
      contract). GGA iterations: (1) []any hard violation → typed params returning literal 9;
      (2) WARNING: circular compile-guarantee claim (unread params) → no-arg len([]string{...}) shape
      per reviewer; (3) RBAC map comment corrected (three tools absent from map, not one —
      check_availability / search_clients_advanced row-scope / search_services_advanced RequireRole,
      all verified downstream); (4) commandKind zero value now fail-secure commandInvalid. Duplicate
      serve-mode ProfessionalsRepo documented. GGA PASSED (final). Commit on `feat/micro-fixes` —
      refactor(mcp): derive startup telemetry counts from the wiring.
      **Follow-ups (pre-existing suggestions, out of scope):** nil-AuthMiddleware panic in
      Server.AuthHandler; per-request Handler() rebuild in methodGate; openCommandDependencies
      hardcoded context.Background().
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

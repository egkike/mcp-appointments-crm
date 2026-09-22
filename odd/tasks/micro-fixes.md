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
- [x] **T6 — Integration coverage**: admin-denial (admin-1 seeded; denied -32001 on all 8 owner-only
      maintenance tools AND admitted on granted get_business_profile → denial is tool-scoped);
      invalid-input through the real mux (TestIntegrationMaintenanceInvalidInput: missing/wrong-type
      args → SDK tool-error envelope with row counts unchanged; malformed body → parse guard HTTP
      400/-32700); tool-count dedup (single expectedToolCount const in test_helpers_test.go, used by
      e2e + both integration suites + the ToolCount registry test — no literal 19 left). All test
      files (excluded from GGA patterns); pipeline green. Commit `8636c4a` on `feat/micro-fixes` —
      test(mcp): admin-denial and invalid-input integration coverage; dedup tool count.
- [x] **T7 — Close-out**: full pre-flight pipeline green (fmt/vet/golangci 0 issues/build/test -race
      all packages) at 8636c4a. Native review gate over the committed branch (base main, committed
      range): lineage review-e463099b71c8f9e1 (high, 4 lenses, 1073 lines, budget 200) — 4/4 lenses
      captured (2 relay flakes recovered via fresh-STATUS retry), refuter confirmed R4-001 CRITICAL
      (resilience, inferential, introduced: strict day-key validator broke persisted legacy
      "01"/"007" keys on read), correction plan 40/200 → correction committed e746220 (38 lines);
      targeted validator then FAILED native-operation-failed (gentle-shell#1324 validator family,
      still open) → lineage ESCALATED (terminal, durable upstream evidence). Owner approved fresh
      START over the corrected candidate: lineage review-f63843adf4e84ecb (high, 4 lenses, 1071
      lines) — 4/4 lenses + refuter, new finding R3-001 CRITICAL (isSingleOwnerViolation delegated
      to the shared registry → future unrelated markers would be misreported as single-owner),
      correction plan 25/200 → correction committed 1e089c9 (25 lines) → targeted validator
      APPROVED, 4 informational findings (R2-001/R2-002/R3-002/R4-001, WARNING, non-blocking
      follow-ups). acknowledge-approved executed; authority burned (review-acknowledged/v1).
      Delivery = ordinary repository policy (issue-first PR, CI, owner squash-merge).

## Commits

(recorded per task on feature branch)
- T1: `cf2bbdc` on `feat/micro-fixes` — fix(domain): strict validation for day keys, HH:MM times and plural day messages
- T2: `f6388bd` — fix(domain): fail closed on nil deps.Bookings and reject midnight-crossing slots
- T3: `0635b57` — refactor(application): split applyProfileUpdates by assignment semantics
- T4: `0850438` — refactor(repository): two-sided guard for 1811 foreign-key classification
- T5: `b18cc59` — refactor(mcp): derive startup telemetry counts from the wiring
- T6: `8636c4a` — test(mcp): admin-denial and invalid-input integration coverage; dedup tool count
- docs: `91ae0cd` — docs(odd): record micro-fixes T6 coverage evidence
- correction R4-001 (gate 1): `e746220` — fix(domain): normalize legacy zero-padded business-hours day keys on read
- correction R3-001 (gate 2): `1e089c9` — fix(repository): keep the single-owner classifier specific to its own marker

## Notes

- Watch: gentle-shell#1324/#924 (validator bug affects correction-heavy reviews;
  workaround = fresh START over corrected candidate).
- Never commit without asking; GGA runs on commit; no --no-verify.
- Review lifecycle owned by PARENT (skip review routing in delegated tasks).

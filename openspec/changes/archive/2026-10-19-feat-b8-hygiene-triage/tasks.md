# Tasks: feat-b8-hygiene-triage

## Phase 1 — Apply (owner decisions already recorded in proposal; no open questions)

- [x] **T1** (S1) · RED test in usecase layer: no-caller ctx → `ErrCodeUnauthenticated` (today RED: Forbidden); wrong-role → Forbidden stays green. Then fix delete_schedule passthrough. · code — **DONE** `44f9018`: TestDeleteScheduleUseCase_Forbidden pins the RequireRole passthrough (RED observed: generic message x3 callers).
- [x] **T2** (S3) · RED test in validator: `15:00:30` vs `15:00` close → exact-to-minute rejection; then add the step-4 seconds guard. · code — **DONE** `44f9018`: sub-minute + nanosecond RED cases, exact-minute differential pin.
- [x] **T3** (S2) · RED test in availability with offset ≠ business zone (next-day local wall clock asserted via validator message); then fix `Start: localStart`. · code — **DONE** `44f9018`: RED `BUSINESS_CLOSED ("martes")` → GREEN out-of-hours at the 18:00 close (pro end 20:00).
- [x] **T4** (S4) · RED tests in entity: duplicate normalization (`"1"`+`"01"`) → error naming both; sole `"01"` still reads; write-path rejects the same shape. Then implement duplicate detection shared by read+write paths. · code — **DONE** `44f9018`: read-path ambiguity error (distinct + identical values); sole `"01"` compat kept; write path transitively rejects (`validateBusinessHoursJSON` rejects zero-padded keys outright).
- [x] **T5** (S5) · cleanup breadcrumbs: delete `stdinSecretLine`/`secretLineReader` plumbing from the three call sites and four function signatures; keep `consoleInput.Secret()`; adjust any hook-stub tests to pipe stdin; run consolehappy-path tests unchanged. · code — **DONE** `44f9018`: behavior-preserving (console suite green untouched).
- [x] **T6** (all) · full Pre-Commit Checklist: `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`; GGA rounds per AGENTS.md delegated/inline conventions. · docs — **DONE**: fmt/vet/lint clean, build ok, -race 15/15 pkgs, govulncheck clean; GGA r1-r2 real findings fixed (f9 log, f9b wording via worker RED→GREEN), r3-r4 ambiguous STRICT parser (#113) → --no-verify owner.
- [x] **T9** (f9) · server-side diagnosis + honest wording: `PurgeInactiveAccounts` logs the masked per-row cause (`purge_log_test.go`), and the console + TUI reports say "Purga parcial" on a failed batch instead of "Purga completa". · code

## Phase 2 — Delivery (owner gates, ordinary repository policy)

- [x] **T7** · single hygiene PR (branch `feat/b8-hygiene-triage`), Conventional Commit, native review is the default gate per AGENTS.md routing (business logic/validator touched); size-note in PR body if line churn inflates. · docs — **DONE**: PR #140 (merged squash `b82e0d9`, CI run 39332483563 success on the merge commit); native review `review-e9d36a7ae2aa589f` medium/review-risk APPROVED + authority burned (gentle-ai.review-acknowledged/v1).
- [x] **T8** · backlog.md B8 row statuses updated per final decisions (f1-f4 CLOSED-by-fix pending merge, f5/F6 f7 disposition-closed; f8 already closed) + Log L15 after merge. · docs — **DONE** post-merge: B8 row CLOSED (see backlog.md Log L16).

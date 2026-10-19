# Tasks: feat-b8-hygiene-triage

## Phase 1 — Apply (owner decisions already recorded in proposal; no open questions)

- [ ] **T1** (S1) · RED test in usecase layer: no-caller ctx → `ErrCodeUnauthenticated` (today RED: Forbidden); wrong-role → Forbidden stays green. Then fix delete_schedule passthrough. · code
- [ ] **T2** (S3) · RED test in validator: `15:00:30` vs `15:00` close → exact-to-minute rejection; then add the step-4 seconds guard. · code
- [ ] **T3** (S2) · RED test in availability with offset ≠ business zone (next-day local wall clock asserted via validator message); then fix `Start: localStart`. · code
- [ ] **T4** (S4) · RED tests in entity: duplicate normalization (`"1"`+`"01"`) → error naming both; sole `"01"` still reads; write-path rejects the same shape. Then implement duplicate detection shared by read+write paths. · code
- [ ] **T5** (S5) · cleanup breadcrumbs: delete `stdinSecretLine`/`secretLineReader` plumbing from the three call sites and four function signatures; keep `consoleInput.Secret()`; adjust any hook-stub tests to pipe stdin; run consolehappy-path tests unchanged. · code
- [ ] **T6** (all) · full Pre-Commit Checklist: `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`; GGA rounds per AGENTS.md delegated/inline conventions. · docs
- [x] **T9** (f9) · server-side diagnosis + honest wording: `PurgeInactiveAccounts` logs the masked per-row cause (`purge_log_test.go`), and the console + TUI reports say "Purga parcial" on a failed batch instead of "Purga completa". · code

## Phase 2 — Delivery (owner gates, ordinary repository policy)

- [ ] **T7** · single hygiene PR (branch `feat/b8-hygiene-triage`), Conventional Commit, native review is the default gate per AGENTS.md routing (business logic/validator touched); size-note in PR body if line churn inflates. · docs
- [ ] **T8** · backlog.md B8 row statuses updated per final decisions (f1-f4 CLOSED-by-fix pending merge, f5/F6 f7 disposition-closed; f8 already closed) + Log L15 after merge. · docs

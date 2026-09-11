# Verify Report: feat-setup-import — Phase 3 (PR 3 of 3)

> **Change:** feat-setup-import
> **Scope:** Phase 3 only (TASK-3.1 → TASK-3.3, wiring + docs)
> **Branch:** `feat-setup-import-wiring` (stacked on `feat-setup-import-seeder` @ fdd373b)
> **Attempt token:** sha256:f2ac489eea5f827db98f26555e006b39c00728841ccbe62e435a119863951b25
> **Request ID:** req-verify-phase3-20260911-01 · **Work-unit:** verify-phase3-wiring
> **skill_resolution:** paths-injected (`golang-patterns`)
> **Date:** 2026-09-11

## Status: COMPLETE (verify clean for the slice — archive NOT ready: parent-owned commit pending)

## Executive Summary

Phase 3 is correctly implemented and fully verified. The diff vs `feat-setup-import-seeder` is exactly **11 insertions across 2 files** (`cmd/mcp-server/main.go`, `docs/installation.md`), matching the minimal-slice claim and sitting at 2.75% of the 400-line review budget. The `config.SeedOnBoot` hook is placed precisely in the single-threaded boot window with the required error wrapping, and the Spanish docs note covers first-boot seeding, guard skip, and runtime-edit safety with no contradictions. Full verification pipeline (build, vet, race tests, golangci-lint, fmt) is green.

## Task Completion

| Task | Status | Evidence |
|------|--------|----------|
| TASK-3.1 | ✅ Verified | Hook inserted in `run()` after `defer database.Close()`, before `// ── Construct repositories` (main.go lines ~146–152) |
| TASK-3.2 | ✅ Verified | Spanish note added after Paso 1 JSON listing (installation.md line 44) |
| TASK-3.3 | ✅ Verified (pipeline) | All commands below green; commit action deferred |

### Unchecked implementation task lines

3 unchecked `- [ ]` lines remain in `tasks.md`. All are **commit actions explicitly documented in `apply-progress.md` as deferred parent lifecycle actions** ("parent owns commit/review/delivery"):

- `- [ ] Commit: feat(config): add setup directory resolver, JSON loader, and business_hours mapping` — **stale checkbox**: commit exists on branch (`f81d333`), proven by git log + apply-progress. Parent may check it off at commit reconciliation.
- `- [ ] Commit: feat(config): add first-boot seeder with transactional guard and rollback` — **stale checkbox**: commit exists on branch (`fdd373b`), proven by git log + apply-progress. Parent may check it off at commit reconciliation.
- `- [ ] Commit: feat(cmd): wire first-boot seed hook and update installation docs` — **genuinely pending**: PR 3 changes are staged-but-uncommitted (`M cmd/mcp-server/main.go`, `M docs/installation.md` on branch `feat-setup-import-wiring`). This is the parent's next action before archive.

**Archive disposition:** archive is NOT ready until the PR 3 commit lands and the stale checkboxes are reconciled. This is a non-critical parent-lifecycle remainder, not an implementation gap — no code, spec, or test work is missing.

## Check Results

### 1. Hook placement — PASS

- Inserted **immediately after** the `defer func() { ... database.Close() }()` block of `db.NewDatabase` and **immediately before** `// ── Construct repositories` (line ~154: `bookingsRepo := repository.NewBookingsRepo(...)`).
- HTTP serving (`mcp.NewServer` → `http.Server`, lines ~240–270) occurs well after repo construction, so the seed executes in the single-threaded window before serving — satisfies "Seed runs before serving" scenario.
- Error path: `return fmt.Errorf("importar configuración inicial: %w", err)` returns from `run()` → fatal, no serving, exit 1 — satisfies "Rollback restores pre-hook behavior" reversibility (pure additive hook).
- Call signature matches `func SeedOnBoot(ctx context.Context, conn *sql.DB, logger *slog.Logger) error` (`internal/config/setup_seeder.go:18`): called as `config.SeedOnBoot(ctx, database.Conn, logger)`. ✅
- Error wrap string is exactly `"importar configuración inicial: %w"` as required. ✅

### 2. Docs wording — PASS

- Note is in Spanish, matching the document's voseo style ("Conservá").
- Covers all three required points: **automatic first-boot seeding when the business profile is empty** ("siembra la base de datos automáticamente en el primer arranque si el perfil del negocio está vacío"), **guard skip** ("En los arranques siguientes la importación se omite gracias al guarda"), and **runtime edits safe** ("las modificaciones que hagas en tiempo de ejecución... no se sobrescriben").
- Replaces the "artefactos finales sin consumidor" implication ("ya no son solo artefactos finales sin consumidor"); repo-wide grep finds no other doc still claiming the JSONs have no consumer.
- Placed after the Paso 1 JSON listing, before Paso 2 deployment — logical reader flow; no contradiction with deployment docs.
- Minor (SUGGESTION, non-blocking): the closing sentence "Conservá los JSON como respaldo o para reprovisionar el sistema; ya no son solo artefactos finales sin consumidor" is slightly awkward — consider "ya que el servidor los consume en el primer arranque". Not a spec violation; the spec scenario only requires the siembra statement + absence of the final-artifact-only claim.

### 3. Verification commands (each as `<command>: <observed result>`)

| Command | Observed result |
|---------|----------------|
| `go build -o /dev/null ./...` | OK — compiled clean (BUILD_OK) |
| `go vet ./...` | OK — no issues (VET_OK) |
| `go test -race ./cmd/mcp-server/...` | PASS — `ok github.com/egkike/mcp-appointments-crm/cmd/mcp-server 1.512s` |
| `go test -race ./...` | PASS — all 11 test packages `ok` (config, cmd, db, mcp, usecase, entity, etc.); no existing tests broken |
| `golangci-lint run ./...` | `0 issues.` exit 0 |
| `go fmt ./...` | Clean — no files reformatted |

### 4. Review budget — PASS

- Slice vs `feat-setup-import-seeder`: `cmd/mcp-server/main.go` +9, `docs/installation.md` +2 = **11 insertions / 0 deletions / 2 files**.
- 11 / 400 = **2.75% of budget** — well within. No chained-PR split needed for this slice beyond the planned PR 3 boundary.

### 5. Out-of-scope isolation — PASS

- `git diff feat-setup-import-seeder --name-only` returns exactly: `cmd/mcp-server/main.go`, `docs/installation.md`.
- Untracked `openspec/changes/feat-setup-import/` is the expected OpenSpec artifact directory (planning artifacts, not runtime code).
- No other repo files touched. Server behavior with the hook reverted is identical to pre-change (rollback boundary respected).

## Structured Status / actionContext Findings

- Native status: `verify: ready`, `sync/archive: blocked` (correct — sync/archive wait on clean verify + commit reconciliation).
- `taskProgress` reports 48/51 with 3 unchecked commit lines — confirmed above as parent-owned/deferred, with 2 provably stale (commits exist) and 1 genuinely pending (PR 3 commit).
- `actionContext.mode: repo-local` with workspace root inside `allowedEditRoots` — ownership proven; verify was read-only except this report file.

## Strict TDD Compliance

Not applicable to this slice: `apply-progress.md` Phase 3 section explicitly states strict TDD was not active for pure wiring + docs on top of the already-tested `config.SeedOnBoot` (Phases 1–2 carry the TDD Cycle Evidence tables and were verified in their own verify rounds). No new testable logic was added in Phase 3; existing full-suite regression coverage is green. No assertion-quality findings (no new tests in this slice).

## Review Workload / PR Boundary

- Chained PRs recommended: **Yes** — confirmed only the assigned PR 3 slice (wiring + docs) was implemented in this work unit.
- `Chain strategy: stacked-to-main` — branch `feat-setup-import-wiring` is stacked on `feat-setup-import-seeder` (fdd373b), matching the plan. PR 3 target: `main` after PR 2 merge.
- `size:exception`: not used, not needed (11 lines ≪ 400).
- No scope creep detected.

## Spec Coverage (setup-seeder REQ 6–7)

- **REQ: Boot wiring in the composition root** — ✅ both scenarios satisfied by hook placement (verified above).
- **REQ: First-boot seeding is documented** — ✅ scenario satisfied ("siembra la DB en el primer arranque si el perfil está vacío"; no final-artifact-only claim remains).

## Exact Blockers (before archive)

1. **Parent-owned commit pending:** `- [ ] Commit: feat(cmd): wire first-boot seed hook and update installation docs` — PR 3 changes are uncommitted on `feat-setup-import-wiring`.
2. **Stale checkbox reconciliation (non-blocking for delivery, blocking for clean archive):** the two Phase 1/2 commit lines can be checked off once the parent confirms commits `f81d333`/`fdd373b` against their apply-progress evidence.

## Risks

- None material in the code. The only residual watch item is the canonical drift warning already recorded in the spec Notes (`business-profile` spec still pins day-name keys vs shipped numeric-string keys) — must be superseded by a `business-profile` delta before/at archive time. Out of Phase 3 scope.

## Next Recommended

1. Parent: commit PR 3 (`feat(cmd): wire first-boot seed hook and update installation docs`) — after RDD gate (default routing; receipt required).
2. Push + open PR 3 against `main` (stacked chain merge order: PR 1 → PR 2 → PR 3).
3. Reconcile stale commit checkboxes, then proceed to sync (`openspec/changes/feat-setup-import/` delta specs) and archive.

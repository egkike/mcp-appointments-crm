# Verify Report — feat-setup-import, Phase 2 (Seeder) — PR 2 of 3

> **Change:** feat-setup-import
> **Scope:** Phase 2 only (TASK-2.1 → TASK-2.4). Phase 3 (TASK-3.x) explicitly NOT verified.
> **Branch:** `feat-setup-import-seeder` (stacked on `feat-setup-import-loader` @ f81d333 — merge-base confirmed)
> **Attempt token:** sha256:42ae5a85f1fd840ad238511891d554d4a273f1df1ecdf164e64efd5e114ebddd
> **skill_resolution:** paths-injected (`golang-patterns`, `go-testing` loaded and applied)
> **Status:** ✅ **PASS (Phase 2 scope)** — no CRITICAL issues in the slice

---

## 1. Verification Commands (run synchronously)

| Command | Observed result |
|---------|-----------------|
| `go test -v -race -count=1 ./internal/config/...` | **PASS** — all tests, incl. `TestSeed_HappyPath`, `TestSeed_ZeroPriceService`, `TestSeed_RollbackOnDuplicateDay`, `TestIsFreshDB`, 4× `TestSeedOnBoot_*`, loader/mapping/resolver suites (`ok ... 1.629s`) |
| `go build -o /dev/null ./...` | **OK** |
| `go vet ./internal/config/...` | **OK** (no issues) |
| `gofmt -l internal/config/` | **Clean** (no output) |
| `golangci-lint run ./internal/config/...` | **0 issues** (exit 0) |

## 2. Spec Conformance — setup-seeder (7 requirements)

Baseline: `specs/setup-seeder/spec.md` (7 requirements, 16 scenarios).

| # | Requirement | Coverage | Verdict |
|---|-------------|----------|---------|
| 1 | **Fresh guard on business_profile.name** | `isFreshDB` mirrors repo lazy-init (`INSERT OR IGNORE ('singleton','')` + `SELECT name`); never inspects `professionals`/`services`. Scenarios: fresh seeds ✅ (`TestSeedOnBoot_FreshValidSeedsAndSecondCallNoOp`), second boot no re-seed ✅ (same test, counts unchanged), manual deletions preserved ✅ (`TestIsFreshDB` — delete all professionals → still not-fresh) | ✅ PASS |
| 2 | **Single atomic transaction** | `seed()`: `BeginTx` → committed-flag + `defer Rollback` → profile UPDATE (`RowsAffected==1` asserted) → professionals → schedules → services → `Commit`. Scenarios: happy path commits ✅; mid-seed failure rolls back ✅ (`TestSeed_RollbackOnDuplicateDay` — real mid-tx UNIQUE failure, asserts `name==''`, 0 professionals, 0 services) | ✅ PASS |
| 3 | **Direct parameterized SQL, not repositories** | All statements bind via `?`; zero concatenation; no repository calls. Scenarios: placeholders ✅ (source review); zero-price service seeds ✅ (`TestSeed_ZeroPriceService`, D5); FTS in sync ✅ (`services_fts MATCH` finds seeded service in happy path) | ✅ PASS |
| 4 | **Singleton upsert, all 18 wizard fields** | `UPDATE ... WHERE id='singleton'`, no second row; `business_hours` stored as mapped numeric-string JSON. Scenarios: profile round-trip ✅ (`assertBusinessProfileFields` — all 18 fields vs wizard values, exactly 1 row); hours drive availability ✅ *compositionally* (DB stores `mapBusinessHours` output byte-for-byte; Phase 1 `TestMapBusinessHours_EntityRoundTrip` proves that exact output → `IsOpenOn(1)`/`GetOpenClose(1)`/`IsOpenOn(7)` correct) — no single end-to-end `GetBusinessProfile`+`IsOpenOn`-on-seeded-DB test, acceptable transitive coverage | ✅ PASS (1 note) |
| 5 | **Boot-time failure policy** | Scenarios: malformed JSON → semantic Spanish error naming file, DB pre-seed state ✅ (`TestSeedOnBoot_FreshMalformedJSONAborts`); missing JSON exits 1 ✅ *at loader level* (`setup_loader_test.go`: `"setup_staff.json" + "no existe"`; the boot path routes through the same `LoadSetup` error) — no dedicated boot-level missing-file test, acceptable; non-fresh ignores bad setup ✅ (`TestSeedOnBoot_NotFreshMissingSetupDirReturnsNil` — guard-first makes this true by construction; tested with missing dir, stronger than spec's malformed-JSON wording since the loader cannot run) | ✅ PASS (2 notes) |
| 6 | **Boot wiring in composition root** | Phase 3 scope (TASK-3.1) — out of scope for this verification | ⏸ DEFERRED (by design) |
| 7 | **First-boot seeding documented** | Phase 3 scope (TASK-3.2) — out of scope for this verification | ⏸ DEFERRED (by design) |

**Summary:** Requirements 1–5 (13 of 16 scenarios) verified in Phase 2 scope; 11 direct, 2 compositional/transitive (notes above). Requirements 6–7 are Phase 3 deliverables by the approved PR chain.

## 3. W1 Disposition Correctness

Confirmed correct and surgical:

- `git diff` of `setup_loader.go` vs base f81d333 = **exactly 5 deleted lines**: the `seenDays` map declaration and its duplicate-day check/insertion. No other change.
- `git diff` of `setup_loader_test.go` = **exactly 7 deleted lines**: the obsolete `"duplicate day in schedule"` validation case. No other change.
- DB `UNIQUE(professional_id, day_of_week)` is now the **sole** duplicate-day enforcer; `TestSeed_RollbackOnDuplicateDay` provably exercises a **real mid-transaction UNIQUE failure** (data passes `validateForSeed` — no Go pre-check can reject it — and the error surfaces via `translateConstraint` → `"ya existe un horario para ese día"`).
- **No other pre-check shadows a DB constraint reserved as a test vector.** `validateForSeed` retains only: name/currency/timezone non-empty, slot interval > 0, payment-method entries, entity `Validate()` defense-in-depth, day range 0–6, HH:MM format, `start < end`. None of these duplicate a UNIQUE constraint; the only reserved vector (schedules UNIQUE) is unobstructed.

## 4. TDD Evidence & Assertion Quality

- `apply-progress.md` contains a Phase 2 `TDD Cycle Evidence` table (RED/GREEN per component: seeder core, constraint translation, boot integration). Cross-referenced: all cited test functions exist in the codebase and pass under `-race -count=1`.
- **Assertion quality: good.** No tautologies, no ghost loops, no type-only assertions. Assertions check concrete DB state (row counts, exact field round-trips, rollback invariants), specific error substrings, and FTS searchability. `assertBusinessProfileFields` compares every nullable column with proper `sql.NullString`/`NullFloat64` semantics — strong, not smoke-only.
- File-based DB per ADR-SD-10, pinned in a test-file comment in both test files (`:memory:` prohibition documented). ✅

## 5. Review Workload / PR Boundary

- **Chain strategy:** `stacked-to-main` confirmed — branch `feat-setup-import-seeder` sits directly on the Phase 1 commit f81d333 (merge-base with `origin/feat-setup-import-loader` = f81d333). Only the assigned Phase 2 slice was implemented. ✅
- **size:exception:** not used — correct (no explicit record needed). ✅
- **Changed lines for this slice (vs base f81d333):** new files 208 (`setup_seeder.go`) + 346 (`setup_seeder_test.go`) + 170 (`setup_boot_test.go`) = 724; modified −12 (W1 removal). **Total ≈ 736 changed lines.**
- ⚠️ **WARNING — budget overage:** 736 lines vs the 400-line review budget → **overage ≈ +336 lines**. This exceeds even the tasks.md Phase 2 estimate (~580). The overage was anticipated at the change level (forecast 1050–1300, `chained PRs: Yes`, risk High) and the chain was already selected per preflight, so this is a known, accepted deviation — not scope creep. Per `ask-on-risk`, the delivery decision (proceed with PR 2 as-is) remains with the parent/user; `size:exception` is NOT implied.

## 6. Out-of-Scope Verification

| Item | Status |
|------|--------|
| `cmd/mcp-server/main.go` | ✅ untouched (empty diff vs f81d333) |
| `docs/` | ✅ untouched |
| `scripts/` | ✅ untouched |
| `internal/db/` (schema) | ✅ untouched |
| `setup_loader.go` modification = ONLY W1 removal | ✅ confirmed (5 lines, §3) |

## 7. Task Checkbox Status

- TASK-2.1, TASK-2.2, TASK-2.3, TASK-2.4: all `[x]`. ✅ No unchecked *implementation* tasks remain in Phase 2.
- Remaining unchecked in `tasks.md` (all **outside Phase 2 implementation scope**):
  - `[ ] Commit: feat(config): add setup directory resolver, JSON loader, and business_hours mapping` (Phase 1 lifecycle — parent-owned, consistent with PR 1 boundary)
  - `[ ] Commit: feat(config): add first-boot seeder with transactional guard and rollback` (Phase 2 lifecycle — **parent owns this commit next**; per session instruction commits are deferred to parent, matching the Phase 1 disposition)
  - TASK-3.1/3.2/3.3 items (Phase 3 scope)
- **Archive is NOT ready** — correctly blocked: Phase 3 unchecked, both commit lifecycle actions pending, and sync pending. No override requested.

## 8. Exact Blockers (for archive, not for this phase)

1. Phase 3 (TASK-3.1 wiring, TASK-3.2 docs, TASK-3.3 final pipeline) not yet implemented.
2. Commit lifecycle actions deferred to parent (PR 1 and PR 2 commits).
3. Canonical drift follow-up before/at archive: `business-profile` delta superseding the day-name/"null" schedule requirement (spec Notes; design RD-7).

## 9. Findings Summary

| Level | Finding |
|-------|---------|
| WARNING | PR 2 slice ≈736 lines > 400 budget (+336). Known/accepted via approved chain; parent decides delivery per `ask-on-risk`. |
| SUGGESTION | Consider a single end-to-end test asserting `GetBusinessProfile` + `IsOpenOn` against a seeded DB (currently covered compositionally). |
| SUGGESTION | Consider a boot-level missing-file test (currently covered at loader level + same error path). |

**No CRITICAL issues in the Phase 2 slice.**

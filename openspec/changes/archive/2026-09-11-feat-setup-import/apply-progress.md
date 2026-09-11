# Apply Progress: feat-setup-import — Phase 1

> Work-unit: phase-1-loader  
> Branch: feat-setup-import-loader  
> Attempt token: sha256:06f24f3f3f369427d0f8060c6f9ef7e70d5a27546889a744dfec55c8eb317b5c  
> Request ID: req-phase1-loader-20260911-01

## Status

Phase 1 implementation complete. All implementation-owned TASK-1.x checkboxes are marked `[x]` in `openspec/changes/feat-setup-import/tasks.md`, except the final commit action which is intentionally deferred to the parent per the session instruction "Do NOT commit — parent owns commits/review/delivery."

## Completed Tasks (TASK-1.1 → TASK-1.7)

| Task | Summary | Evidence |
|------|---------|----------|
| TASK-1.1 | Setup JSON structs (`setup_types.go`) | File created; `go build ./internal/config/...` passes; no `any`/`interface{}` fields |
| TASK-1.2 | Per-OS setup directory resolver (`setup_resolver.go`) | File created; mirrors `install.sh resolve_paths` minus symlink checks; `MCP_SETUP_DIR` precedence; `filepath.Join`/`filepath.Clean` |
| TASK-1.3 | Resolver unit tests (`setup_resolver_test.go`) | Table-driven over `(goos, env)`; 8 cases; all pass |
| TASK-1.4 | Golden test fixtures (`testdata/`) | `setup_business.json`, `setup_staff.json`, `setup_services.json` created; decode verified |
| TASK-1.5 | Loader + business_hours mapping (`setup_loader.go`) | `LoadSetup`, `readFileCapped`, `mapBusinessHours`, `validateForSeed`; 1 MiB cap; semantic Spanish errors; read-only |
| TASK-1.6 | Loader + mapping unit tests | `setup_loader_test.go`, `setup_mapping_test.go`; happy/missing/malformed/oversized/read-only + mapping round-trip |
| TASK-1.7 | Phase 1 verification gate | `go fmt`, `go vet`, `go build`, `go test -race`, `golangci-lint` all clean |

## Files Created

- `internal/config/setup_types.go`
- `internal/config/setup_resolver.go`
- `internal/config/setup_resolver_test.go`
- `internal/config/setup_loader.go`
- `internal/config/setup_loader_test.go`
- `internal/config/setup_mapping_test.go`
- `internal/config/testdata/setup_business.json`
- `internal/config/testdata/setup_staff.json`
- `internal/config/testdata/setup_services.json`

## Files Modified

- `openspec/changes/feat-setup-import/tasks.md` — Phase 1 checkboxes updated

## TDD Cycle Evidence

Strict TDD was active. Each behavioral component was driven through RED → GREEN with focused failing tests first, then implementation.

| Component | RED (failing test) | GREEN (passing impl) | Notes |
|-----------|-------------------|----------------------|-------|
| Resolver | `TestResolveSetupDirOS` failed to compile because `resolveSetupDirOS` did not exist | `setup_resolver.go` added; all 8 table cases pass | goos/getenv injection makes per-OS logic testable on any host |
| Loader | `TestLoadSetup_*` failed to compile because `LoadSetup` did not exist; mapping tests failed because `mapBusinessHours` did not exist | `setup_loader.go` added; happy/missing/malformed/oversized/read-only cases pass | Read-only assertion verified via content + ModTime comparison |
| Mapping | `TestMapBusinessHours_*` failed because `mapBusinessHours` did not exist | All 8 mapping cases + entity round-trip + determinism pass | `monday`→`"1"`, `sunday`→`"7"`, null→absent, `IsOpenOn`/`GetOpenClose` round-trip |
| Validation | `TestValidateForSeed` added as triangulation after loader/mapping GREEN | Validates empty name, negative price, bad `is_active`, duplicate schedule day | Pre-tx defense-in-depth checks confirmed |

## Verification Results

Commands run synchronously with observed results:

| Command | Result |
|---------|--------|
| `go fmt ./internal/config/...` | OK (no changes) |
| `go vet ./internal/config/...` | OK (no issues) |
| `go build -o /dev/null ./...` | OK |
| `go test -v -race ./internal/config/...` | PASS (all tests) |
| `golangci-lint run ./internal/config/...` | 0 issues |

## Design Deviations

Phase 1 initially reported "None"; Phase 2 verify disclosed **W1** — the Go-side duplicate-day pre-check in `validateForSeed` (`seenDays`) conflicted with ADR-SD-8 §8.3 item 9, which reserved the DB `UNIQUE(professional_id, day_of_week)` constraint as the mid-transaction rollback vector for the TASK-2.2(c) test. Disposition applied in Phase 2:

- Removed the `seenDays` duplicate-day pre-check from `setup_loader.go`.
- Kept day range (0–6), HH:MM format, and start<end checks.
- The DB UNIQUE constraint is now the sole duplicate-day enforcer; its violation is translated to `"ya existe un horario para ese día"` via `translateConstraint` in `setup_seeder.go`.
- Updated `setup_loader_test.go` to drop the obsolete duplicate-day validation case (now covered by the seeder rollback test).

## Remaining Work

Phase 2 (seeder) and Phase 3 (wiring + docs) are intentionally untouched in this work unit:

- `internal/config/setup_seeder.go` (TASK-2.1)
- `internal/config/setup_seeder_test.go` (TASK-2.2)
- `internal/config/setup_boot_test.go` (TASK-2.3)
- `cmd/mcp-server/main.go` seed hook (TASK-3.1)
- `docs/installation.md` update (TASK-3.2)
- Final project-wide pipeline verification (TASK-3.3)

## Workload / PR Boundary

This work unit corresponds to **PR 1 of 3** in the approved stacked-to-main chain (`feat-setup-import-loader`). It is self-contained: no DB dependency, no boot-path changes, no modifications outside `internal/config/` and testdata.

## Deferred Parent Lifecycle Action

- `[ ] Commit: feat(config): add setup directory resolver, JSON loader, and business_hours mapping` — intentionally left unchecked; parent owns commit/review/delivery per session instruction.

---

# Apply Progress: feat-setup-import — Phase 2

> Work-unit: phase-2-seeder  
> Branch: feat-setup-import-seeder  
> Stacked on: feat-setup-import-loader (f81d333)  
> Attempt token: sha256:3a8f48e858380889e4547bdf7a03537b6972e7f89518d564d24108d5168b78af  
> Request ID: req-phase2-seeder-20260911-01

## Status

Phase 2 implementation complete. All implementation-owned TASK-2.x checkboxes are marked `[x]` in `openspec/changes/feat-setup-import/tasks.md`, except the final commit action which is intentionally deferred to the parent per the session instruction "Do NOT commit — parent owns commits/review/delivery."

## Completed Tasks (TASK-2.1 → TASK-2.4)

| Task | Summary | Evidence |
|------|---------|----------|
| TASK-2.1 | Fresh guard + seeder (`setup_seeder.go`) | `SeedOnBoot`, `isFreshDB`, `seed`, `translateConstraint` implemented; all SQL uses `?` placeholders; semantic Spanish errors |
| TASK-2.2 | Seeder unit tests (`setup_seeder_test.go`) | Happy path, zero-price service, rollback on duplicate day, fresh-guard behavior; file-based temp DB |
| TASK-2.3 | Boot integration tests (`setup_boot_test.go`) | End-to-end `SeedOnBoot`: fresh+valid, fresh+malformed, not-fresh+missing dir, fresh+unset HOME |
| TASK-2.4 | Phase 2 verification gate | `go fmt`, `go vet`, `go build`, `go test -race`, `golangci-lint` all clean for `internal/config/...` |

## Files Created

- `internal/config/setup_seeder.go`
- `internal/config/setup_seeder_test.go`
- `internal/config/setup_boot_test.go`

## Files Modified

- `internal/config/setup_loader.go` — W1 disposition: removed Go-side duplicate-day pre-check (`seenDays`)
- `internal/config/setup_loader_test.go` — removed obsolete duplicate-day validation test case
- `openspec/changes/feat-setup-import/tasks.md` — TASK-2.x checkboxes updated
- `openspec/changes/feat-setup-import/apply-progress.md` — this Phase 2 section appended; Phase 1 Design Deviations corrected for W1

## TDD Cycle Evidence

Strict TDD was active. Each behavioral component was driven through RED → GREEN with focused failing tests first, then implementation.

| Component | RED (failing test) | GREEN (passing impl) | Notes |
|-----------|-------------------|----------------------|-------|
| Seeder core | `setup_seeder_test.go` failed to compile because `seed`, `isFreshDB` did not exist | `setup_seeder.go` added; happy path, zero-price, rollback, guard tests pass | Direct SQL tx; `committed` flag + defer rollback |
| Constraint translation | Rollback test expected `"ya existe un horario para ese día"` | `translateConstraint` maps schedules UNIQUE violation | Real DB mid-tx failure vector thanks to W1 |
| Boot integration | `setup_boot_test.go` failed to compile because `SeedOnBoot` did not exist | `SeedOnBoot` orchestrator added; all 4 boot scenarios pass | Guard-first ordering verified: not-fresh + missing dir returns nil |

## Verification Results

Commands run synchronously with observed results:

| Command | Result |
|---------|--------|
| `go fmt ./internal/config/...` | OK (no changes) |
| `go vet ./internal/config/...` | OK (no issues) |
| `go build -o /dev/null ./...` | OK |
| `go test -v -race ./internal/config/...` | PASS (all tests) |
| `golangci-lint run ./internal/config/...` | 0 issues |
| `go test -v -race ./...` | PASS (no existing tests broken) |

## Design Deviations

- **W1 (mandatory disposition from Phase 1 verify):** Removed the Go-side duplicate-day pre-check from `validateForSeed`; the DB `UNIQUE(professional_id, day_of_week)` constraint is now the sole duplicate-day enforcer, with its violation translated to `"ya existe un horario para ese día"`. This corrects the Phase 1 apply-progress note that inaccurately listed "None" under Design Deviations.

## Remaining Work

Phase 3 (wiring + docs) is intentionally untouched in this work unit:

- `cmd/mcp-server/main.go` seed hook (TASK-3.1)
- `docs/installation.md` update (TASK-3.2)
- Final project-wide pipeline verification (TASK-3.3)

## Workload / PR Boundary

This work unit corresponds to **PR 2 of 3** in the approved stacked-to-main chain (`feat-setup-import-seeder`). It builds on Phase 1 (`feat-setup-import-loader`) and is scoped to `internal/config/` only, plus the W1 surgical edits required to make the rollback test exercise the real DB constraint.

## Deferred Parent Lifecycle Action

- `[ ] Commit: feat(config): add first-boot seeder with transactional guard and rollback` — intentionally left unchecked; parent owns commit/review/delivery per session instruction.

---

# Apply Progress: feat-setup-import — Phase 3

> Work-unit: phase-3-wiring
> Branch: feat-setup-import-wiring
> Stacked on: feat-setup-import-seeder (fdd373b)
> Attempt token: sha256:762cb32ad945c71b08dc6dd3bfc3b53ba40cb2856133d00a26136d493fbdc01b
> Request ID: req-phase3-wiring-20260911-01

## Status

Phase 3 implementation complete. All implementation-owned TASK-3.x checkboxes are marked `[x]` in `openspec/changes/feat-setup-import/tasks.md`, except the final commit action which is intentionally deferred to the parent per the session instruction "Do NOT commit — parent owns commits/review/delivery."

## Completed Tasks (TASK-3.1 → TASK-3.3)

| Task | Summary | Evidence |
|------|---------|----------|
| TASK-3.1 | Wire `SeedOnBoot` in `cmd/mcp-server/main.go` | Added `internal/config` import; inserted 10-line hook immediately after `defer database.Close()`, before `// ── Construct repositories`; error wrapped as `importar configuración inicial: %w` |
| TASK-3.2 | Update `docs/installation.md` | Added Spanish note after Paso 1 JSON listing: automatic first-boot seed, guard skips subsequent boots, runtime edits safe, JSONs are consumidos by the server |
| TASK-3.3 | Final pipeline verification | `go fmt`, `go vet`, `golangci-lint`, `go build`, and full `go test -race ./...` all clean |

## Files Modified

- `cmd/mcp-server/main.go`
- `docs/installation.md`
- `openspec/changes/feat-setup-import/tasks.md`
- `openspec/changes/feat-setup-import/apply-progress.md` (this section)

## TDD Cycle Evidence

Strict TDD was not active for this slice; the work is pure wiring and documentation on top of already-tested `config.SeedOnBoot`. The existing test suite provides regression coverage.

## Verification Results

Commands run synchronously with observed results:

| Command | Result |
|---------|--------|
| `go fmt ./...` | OK (no changes) |
| `go vet ./...` | OK (no issues) |
| `go build -o /dev/null ./...` | OK |
| `go test -v -race ./...` | PASS (all tests, full suite) |
| `golangci-lint run ./...` | 0 issues |

## Design Deviations

None. The hook placement matches design §10 exactly: immediately after the `defer database.Close()` block and before repository construction.

## Remaining Work

- Parent-owned commit: `feat(cmd): wire first-boot seed hook and update installation docs`

## Workload / PR Boundary

This work unit corresponds to **PR 3 of 3** in the approved stacked-to-main chain (`feat-setup-import-wiring`). It is a minimal wiring + docs slice (~20 changed lines) stacked on `feat-setup-import-seeder`.

## Deferred Parent Lifecycle Action

- `[ ] Commit: feat(cmd): wire first-boot seed hook and update installation docs` — intentionally left unchecked; parent owns commit/review/delivery per session instruction.

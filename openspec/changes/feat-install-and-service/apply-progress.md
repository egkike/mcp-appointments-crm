# Apply Progress — feat-install-and-service

**Change:** feat-install-and-service
**Phase:** 5 — apply (PR1 merged, PR2 complete)
**Status:** PR1 T1 merged to main (`a52d1cf`); PR2 T2+T3 complete, ready for verify
**Executor:** sdd-apply (+ orchestrator rebase resolution)
**Updated:** 2026-09-06

---

## Completed Tasks

### PR1 (merged as `a52d1cf`)

- [x] T1.1 RED: crear `cmd/mcp-server/main_test.go` con tests para `--version`
- [x] T1.2 GREEN: implementar guard `--version` en `cmd/mcp-server/main.go`
- [x] T1.3 VERIFY: `go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...`

### PR2 (this branch, rebased onto main post-PR1-merge)

- [x] T2.1 — `setup/service/mcp-appointments-crm.service`
- [x] T2.2 — `setup/service/com.mcp.appointments.server.plist`
- [x] T2.3 — `setup/service/nssm-install.md`
- [x] T2.4 — VERIFY templates (3 files + grep checks)
- [x] T3.1 — RED: `scripts/tests/backup_test.sh` (5 tests, RED before implementation)
- [x] T3.2 — GREEN: `scripts/backup.sh`
- [x] T3.3 — VERIFY: `bash scripts/tests/run_tests.sh` all green

Parent-owned / deferred tasks (not touched):

- T4/T5 (install deploy pipeline) — PR3, size:exception aprobado
- T6/T7 (docs + final verify) — PR4

---

## Files Changed

### PR1 (in main)

| File | Action | Lines |
|------|--------|-------|
| `cmd/mcp-server/main_test.go` | New test file | ~95 |
| `cmd/mcp-server/main.go` | Extend `main()` with `--version` guard + pure helpers | ~20 insertions |

**Total changed lines (PR1):** ~115 additions — within 400-line budget.

### PR2 (this branch)

| File | Action | Lines (aprox) |
|---|---|---|
| `setup/service/mcp-appointments-crm.service` | new | ~17 |
| `setup/service/com.mcp.appointments.server.plist` | new | ~23 |
| `setup/service/nssm-install.md` | new | ~36 |
| `scripts/backup.sh` | new | ~57 |
| `scripts/tests/backup_test.sh` | new | ~128 |
| `openspec/changes/feat-install-and-service/tasks.md` | checkbox updates T2/T3 | ~7 lines |
| `openspec/changes/feat-install-and-service/apply-progress.md` | updated (PR1+PR2 combined on rebase) | — |

Files intentionally NOT modified:

- `cmd/mcp-server/main.go` (PR1 scope, in main)
- `scripts/install.sh` (PR3 scope)
- `scripts/tests/install_validators_test.sh`
- `scripts/tests/install_e2e_test.sh`
- `scripts/tests/run_tests.sh` (no change needed; auto-discovers `backup_test.sh`)

---

## Test Commands Run

### PR1

```bash
# Safety net / focused package test
go test -v -race ./cmd/mcp-server/...

# Full quality gates (T1.3)
go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...
```

**Result:** All green. No `gofmt` diff, no `go vet` warnings, full test suite passes.

### PR2

```bash
# Baseline / safety net
bash scripts/tests/run_tests.sh
# => 2/2 suites pasaron (install_e2e_test.sh, install_validators_test.sh)

# RED cycle for backup
bash scripts/tests/backup_test.sh
# => FAILED (backup.sh no existe / exit 127)

# GREEN cycle for backup
chmod +x scripts/backup.sh
bash scripts/tests/backup_test.sh
# => OK (5 tests)

# Final regression gate
bash scripts/tests/run_tests.sh
# => OK: 3/3 suites pasaron (backup_test.sh, install_e2e_test.sh, install_validators_test.sh)
```

---

## TDD Cycle Evidence

Strict TDD active (`openspec/config.yaml` → `rules.apply.tdd: true`, `test_command: go test -v -race ./...`).

### PR1

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T1.1 | `cmd/mcp-server/main_test.go` | Unit + Exec | N/A (no prior tests in package) | ✅ Written (undefined `wantsVersion`/`printVersion`) | ✅ Passed | ✅ 4 cases for `wantsVersion` + 1 output test + 1 exec-based binary test | ✅ Extracted pure `wantsVersion` and `printVersion`; `main()` delegates to `os.Stdout` |

- **Total tests written:** 3 top-level tests (6 subtests + 1 exec test)
- **Pure functions created:** 2 (`wantsVersion`, `printVersion`)
- `--version` guard runs **before** `run()` — no DB/network side effects. No `flag` package. Zero changes to `internal/mcp/config.go`, `internal/db/`, or DB defaults.

### PR2

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| T2.1 | n/a (template) | Structural | N/A (new file) | n/a | n/a | ➖ Single canonical unit | ➖ None needed |
| T2.2 | n/a (template) | Structural | N/A (new file) | n/a | n/a | ➖ Single canonical plist | ➖ None needed |
| T2.3 | n/a (document) | Structural | N/A (new file) | n/a | n/a | ➖ Single Spanish placeholder | ➖ None needed |
| T2.4 | `scripts/tests/run_tests.sh` + grep | Verify | ✅ 2/2 baseline | n/a | n/a | ➖ ls + grep assertions | ➖ None needed |
| T3.1 | `scripts/tests/backup_test.sh` | Unit (shunit2) | ✅ 2/2 baseline | ✅ Written | n/a | ✅ 5 scenarios | n/a (test only) |
| T3.2 | `scripts/tests/backup_test.sh` | Unit (shunit2) | ✅ 2/2 baseline | ✅ Referenced missing `backup.sh` | ✅ Passed | ✅ 5 scenarios force real logic | ➖ Clean as written |
| T3.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 2/2 baseline | n/a | ✅ 3/3 suites | n/a | ➖ None needed |

- **Total tests written**: 5 (in `backup_test.sh`), 5/5 passing
- `backup.sh` uses `TMP` and `TMP_GZ` cleaned by a single `trap` (slightly more robust than the design snippet). Contract identical.
- `nssm-install.md` includes the explicit Windows-automation caveat and `%APPDATA%` layout per D4/REQ-SU-004.

---

## Design Deviations

None. PR1 matches design §5.4; PR2 follows design §5.2 (backup) and §5.3 (service templates) verbatim.

---

## Remaining Work

- PR3: T4 + T5 (`scripts/install.sh` deploy pipeline + `scripts/tests/install_deploy_test.sh`) + CRITICAL-1 fix (backup.sh persistent install)
- PR4: T6 + T7 (`docs/installation.md`, `docs/maintenance.md`, alignment edits, final verify)

---

## PR Boundary

- **PR1:** T1 only — merged (`a52d1cf`), RDD receipt burned (`review-5cb3536d01cf8d90`).
- **PR2 (this branch):** T2 + T3 only (~330 LOC, within 400-line budget, no `size:exception`).
- **Rebase note (2026-09-06):** rebased onto main post-PR1-merge; conflict on this file (add/add) resolved by combining PR1 + PR2 sections. No functional change.

# Apply Progress — feat-install-and-service / PR3

**Change:** feat-install-and-service
**PR:** PR3 de 4 — `feat/install-deploy-pipeline` (size:exception aprobado)
**Status:** implementation complete, ready for verify
**Executor:** sdd-apply
**Date:** 2026-09-06

---

## Structured Status

```yaml
schemaName: spec-driven
changeName: feat-install-and-service
artifactStore: openspec
planningHome:
  root: /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm
  changesDir: openspec/changes
changeRoot: openspec/changes/feat-install-and-service
artifactPaths:
  proposal: [openspec/changes/feat-install-and-service/proposal.md]
  specs:
    - openspec/changes/feat-install-and-service/specs/install-service/spec.md
    - openspec/changes/feat-install-and-service/specs/service-units/spec.md
    - openspec/changes/feat-install-and-service/specs/binary-version/spec.md
    - openspec/changes/feat-install-and-service/specs/install-docs/spec.md
    - openspec/changes/feat-install-and-service/specs/backup/spec.md
  design: [openspec/changes/feat-install-and-service/design.md]
  tasks: [openspec/changes/feat-install-and-service/tasks.md]
  applyProgress: [openspec/changes/feat-install-and-service/apply-progress.md]
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
taskProgress:
  total: 7
  complete: 5
  remaining: 2
  unchecked:
    - T6.1 — Write docs/installation.md
    - T6.2 — Write docs/maintenance.md
    - T6.3 — Align docs/deployment.md and docs/PRD.md
    - T7.1 — Go quality gates
    - T7.2 — Shell test suites
    - T7.3 — File inventory check
    - T7.4 — Manual VM checklist
applyState: ready
dependencies:
  apply: all_done
  verify: ready
  sync: not_applicable
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm
  allowedEditRoots:
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/scripts/install.sh
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/scripts/tests/install_deploy_test.sh
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/scripts/tests/run_tests.sh
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/openspec/changes/feat-install-and-service/tasks.md
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/openspec/changes/feat-install-and-service/apply-progress.md
  warnings: []
nextRecommended: sdd-verify
isNonAuthoritative: false
```

---

## Completed Tasks (with persisted checkbox updates)

PR2 (base, already merged into this branch):

- [x] T2.1 — `setup/service/mcp-appointments-crm.service`
- [x] T2.2 — `setup/service/com.mcp.appointments.server.plist`
- [x] T2.3 — `setup/service/nssm-install.md`
- [x] T2.4 — VERIFY templates
- [x] T3.1 — RED: `scripts/tests/backup_test.sh`
- [x] T3.2 — GREEN: `scripts/backup.sh`
- [x] T3.3 — VERIFY: all suites green

PR3 (this apply):

- [x] T4.1 — RED: `scripts/tests/install_deploy_test.sh` (core functions)
- [x] T4.2 — GREEN: implement core deploy functions in `scripts/install.sh`
- [x] T4.3 — VERIFY: core deploy tests + no regression
- [x] T5.1 — RED: extend `scripts/tests/install_deploy_test.sh` (service + verify + summary)
- [x] T5.2 — GREEN: implement service registration, verification, summary
- [x] T5.3 — VERIFY: all suites green, no regression

Parent-owned / deferred tasks (not touched):

- T6.1/T6.2/T6.3 — docs (`docs/installation.md`, `docs/maintenance.md`, alignment)
- T7.1/T7.2/T7.3/T7.4 — final verification gates

---

## Files Changed

| File | Action | Lines (aprox) |
|---|---|---|
| `scripts/install.sh` | extend (deploy pipeline) | +450 / -9 (diff stat) |
| `scripts/tests/install_deploy_test.sh` | new | ~378 |
| `openspec/changes/feat-install-and-service/tasks.md` | checkbox updates T4/T5 | ~6 lines |
| `openspec/changes/feat-install-and-service/apply-progress.md` | cumulative progress | replaced |

Files intentionally NOT modified:

- `cmd/mcp-server/main.go` (PR1 scope)
- `setup/service/*` (PR2 scope)
- `scripts/backup.sh` (PR2 scope)
- `scripts/tests/install_validators_test.sh`
- `scripts/tests/install_e2e_test.sh`
- `scripts/tests/run_tests.sh` (auto-discovers `install_deploy_test.sh`)

---

## Test Commands Run

```bash
# RED cycle for deploy pipeline
bash scripts/tests/install_deploy_test.sh
# => FAILED (functions did not exist yet)

# GREEN cycle (iterative)
bash scripts/tests/install_deploy_test.sh
# => OK (23 tests)

# Regression gate
bash scripts/tests/run_tests.sh
# => OK: 4/4 suites pasaron (backup_test.sh, install_deploy_test.sh, install_e2e_test.sh, install_validators_test.sh)

# Go quality gates (no Go changes in PR3)
go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...
# => all green
```

---

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| T4.1 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ validators + e2e baseline | ✅ Written | n/a | ✅ 12+ scenarios | n/a (test only) |
| T4.2 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ validators + e2e baseline | ✅ Referenced missing functions | ✅ Passed | ✅ Fixtures for SHA256, paths, env, asset name | ✅ Bash 3.2-safe; trap extended for `DEPLOY_TMP` |
| T4.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 4/4 suites | n/a | ✅ 4/4 suites | n/a | ➖ None needed |
| T5.1 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ T4 green | ✅ Extended failing tests | n/a | ✅ service render, verify, summary, TTY guard | n/a (test only) |
| T5.2 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ T4 green | ✅ Service/verify/summary missing | ✅ Passed | ✅ Linux systemd + macOS launchd paths; archive/repo fallback | ✅ `$0` aware TTY guard preserves piped-test setup |
| T5.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 4/4 suites | n/a | ✅ 4/4 suites | n/a | ➖ None needed |

### Test Summary

- **Total tests written**: 23 (in `install_deploy_test.sh`)
- **Total tests passing**: 23 / 23
- **Layers used**: Unit (23)
- **Approval tests**: None
- **Pure functions created**: `compose_asset_name`, `validate_tag`, `sha256_file`, `render_systemd_unit`, `render_launchd_plist`

---

## Deviations from Design

1. **`run_setup_guard_tty` checks `$0` in addition to `[ -t 0 ]`.**
   The literal contract in tasks says `[ -t 0 ]`, but applying only that check broke the existing `install_e2e_test.sh` suite, which legitimately pipes answers into `bash scripts/install.sh` (script executed from a file, `$0` ≠ `bash`). A true `curl ... | bash -s` invocation has `$0 == bash` and no TTY, so the guard still blocks it exactly as D6 requires. Documented in a code comment.

2. **`cleanup_tmp` was extended to remove `DEPLOY_TMP`.**
   The existing trap only cleaned `CURRENT_TMP` (the atomic-write temp file). The deploy pipeline needs a temporary directory for downloads/extraction. The extension preserves the existing `CURRENT_TMP` behavior and adds cleanup for `DEPLOY_TMP`, ensuring no partial binary is left on SHA256 failure.

3. **`render_systemd_unit` compares `DATA_DIR` against the canonical default `$HOME/.local/share/mcp-appointments-crm`.**
   This matches D9: the literal `%h/.local/share/...` is preserved when no XDG override exists; only a custom `XDG_DATA_HOME` triggers absolute-path substitution.

No other deviations.

---

## Remaining Work

- PR4: T6 (`docs/installation.md`, `docs/maintenance.md`, alignment edits) + T7 (final Go/shell/manual verification)

Unchecked task lines from `tasks.md`:

- [ ] T6.1 — Write `docs/installation.md`
- [ ] T6.2 — Write `docs/maintenance.md`
- [ ] T6.3 — Align `docs/deployment.md` and `docs/PRD.md`
- [ ] T7.1 — Go quality gates
- [ ] T7.2 — Shell test suites
- [ ] T7.3 — File inventory check
- [ ] T7.4 — Manual VM checklist

---

## PR Boundary

**PR3 branch suggestion:** `feat/install-deploy-pipeline`
**Scope:** T4 + T5 only — deploy pipeline core + service registration + verification + summary + tests.
**Stacked-to-main:** Branch apunta a `main` (PR1 y PR2 ya mergeados en base).
**Size:** ~840 changed lines (450 additions in `install.sh`, 378 new test lines, plus tasks/progress updates). Exceeds the 400-line budget; `size:exception` was approved by the user because the deploy pipeline and its test matrix form a single cohesive deliverable — an honest split would leave an intermediate PR without SHA256-to-service wiring.

---

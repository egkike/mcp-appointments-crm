# Apply Progress — feat-install-and-service / PR2

**Change:** feat-install-and-service
**PR:** PR2 de 4 — `feat/service-templates-and-backup`
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
  complete: 7
  remaining: 0
  unchecked: []
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
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/setup/service
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/scripts/backup.sh
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/scripts/tests/backup_test.sh
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/scripts/tests/run_tests.sh
  warnings: []
nextRecommended: sdd-verify
isNonAuthoritative: false
```

---

## Completed Tasks (with persisted checkbox updates)

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

| File | Action | Lines (aprox) |
|---|---|---|
| `setup/service/mcp-appointments-crm.service` | new | ~17 |
| `setup/service/com.mcp.appointments.server.plist` | new | ~23 |
| `setup/service/nssm-install.md` | new | ~36 |
| `scripts/backup.sh` | new | ~57 |
| `scripts/tests/backup_test.sh` | new | ~128 |
| `openspec/changes/feat-install-and-service/tasks.md` | checkbox updates T2/T3 | ~7 lines |
| `openspec/changes/feat-install-and-service/apply-progress.md` | new | — |

Files intentionally NOT modified:

- `cmd/mcp-server/main.go` (PR1 scope)
- `scripts/install.sh` (PR3 scope)
- `scripts/tests/install_validators_test.sh`
- `scripts/tests/install_e2e_test.sh`
- `scripts/tests/run_tests.sh` (no change needed; auto-discovers `backup_test.sh`)

---

## Test Commands Run

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

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| T2.1 | n/a (template) | Structural | N/A (new file) | n/a | n/a | ➖ Single canonical unit | ➖ None needed |
| T2.2 | n/a (template) | Structural | N/A (new file) | n/a | n/a | ➖ Single canonical plist | ➖ None needed |
| T2.3 | n/a (document) | Structural | N/A (new file) | n/a | n/a | ➖ Single Spanish placeholder | ➖ None needed |
| T2.4 | `scripts/tests/run_tests.sh` + grep | Verify | ✅ 2/2 baseline | n/a | n/a | ➖ ls + grep assertions | ➖ None needed |
| T3.1 | `scripts/tests/backup_test.sh` | Unit (shunit2) | ✅ 2/2 baseline | ✅ Written | n/a | ✅ 5 scenarios | n/a (test only) |
| T3.2 | `scripts/tests/backup_test.sh` | Unit (shunit2) | ✅ 2/2 baseline | ✅ Referenced missing `backup.sh` | ✅ Passed | ✅ 5 scenarios force real logic | ➖ Clean as written |
| T3.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 2/2 baseline | n/a | ✅ 3/3 suites | n/a | ➖ None needed |

### Test Summary

- **Total tests written**: 5 (in `backup_test.sh`)
- **Total tests passing**: 5 / 5
- **Layers used**: Unit (5)
- **Approval tests**: None — no refactoring tasks
- **Pure functions created**: 0 (script is procedural; behavior asserted via side effects and output)

---

## Deviations from Design

None. The implementation follows design §5.2 (backup) and §5.3 (service templates) verbatim.

Notes:

- `backup.sh` uses `TMP` and `TMP_GZ` variables cleaned by a single `trap`, which is slightly more robust than the design snippet that only removed `$TMP`. The contract (snapshot + gzip + atomic mv) is identical.
- `nssm-install.md` includes the explicit Windows-automation caveat and the `%APPDATA%` layout per D4/REQ-SU-004.

---

## Remaining Work

- PR3: T4 + T5 (`scripts/install.sh` deploy pipeline + `scripts/tests/install_deploy_test.sh`)
- PR4: T6 + T7 (`docs/installation.md`, `docs/maintenance.md`, alignment edits, final verify)

---

## PR Boundary

**PR2 branch suggestion:** `feat/service-templates-and-backup`
**Scope:** T2 + T3 only (~330 LOC, dentro del presupuesto de 400 líneas).
**Stacked-to-main:** Branch apunta a `main` (PR1 ya mergeado como docs + PR #62).
**Size:** within budget; no `size:exception` needed.

# Apply Progress — feat-install-and-service / PR3 — CRITICAL-1 fix

**Change:** feat-install-and-service
**PR:** PR3 de 4 — `feat/install-deploy-pipeline` (size:exception aprobado)
**Status:** CRITICAL-1 fix applied, ready for re-verify
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
applyState: all_done
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
    - /home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm/openspec/changes/feat-install-and-service/apply-progress.md
  warnings: []
nextRecommended: sdd-verify
isNonAuthoritative: false
```

---

## CRITICAL-1 fix — persistent backup.sh + summary path

### Diagnóstico (verify-report)

- `run_deploy()` nunca copiaba `backup.sh` a `$DATA_DIR/scripts/`.
- `_post_install_backup_cmd()` prefería `$EXTRACT_DIR/scripts/backup.sh` (borrado por el trap) y luego `$SCRIPT_DIR/backup.sh` (inexistente en VPS piped).
- Resultado: el comando copy-pasteable del summary no era ejecutable post-deploy (DoD 12/13).

### Cambios aplicados

1. **`install_backup_script()`** en `scripts/install.sh` (sección Deploy pipeline, después de `install_binary`):
   - Fuente por orden: `$EXTRACT_DIR/scripts/backup.sh` → fallback `$SCRIPT_DIR/backup.sh`.
   - Destino: `$DATA_DIR/scripts/backup.sh`.
   - `mkdir -p` 0700 + copia atómica tmp + `mv` + `chmod 0755`.
   - Error claro en español si ninguna fuente existe.
2. **Wire en `run_deploy()`**: `install_backup_script` se ejecuta después de `install_binary` y antes de `install_service`.
3. **`_post_install_backup_cmd()`** ahora prefiere `$DATA_DIR/scripts/backup.sh` cuando existe, manteniendo los fallbacks anteriores.

### Tests agregados

- `test_install_backup_script_from_extract`
- `test_install_backup_script_fallback_script_dir`
- `test_install_backup_script_no_source_fails`
- `test_post_install_backup_cmd_prefers_persistent`

Se actualizaron `setUp()`/`tearDown()` para salvar/restaurar `SCRIPT_DIR` y `EXTRACT_DIR` entre tests y evitar contaminación cruzada.

### Files changed in this fix

| File | Action | Lines (diff stat) |
|---|---|---|
| `scripts/install.sh` | add `install_backup_script`, wire in `run_deploy`, fix `_post_install_backup_cmd` | +39 / -1 |
| `scripts/tests/install_deploy_test.sh` | add 4 backup-install tests + setUp/tearDown save/restore | +71 |
| `openspec/changes/feat-install-and-service/apply-progress.md` | cumulative progress | updated |

### Verificación

```bash
bash scripts/tests/install_deploy_test.sh
# => OK (27 tests)

bash scripts/tests/run_tests.sh
# => OK: 4/4 suites pasaron

go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...
# => all green

git diff --stat
#  scripts/install.sh                   | 39 +++++++++++++++++++-
#  scripts/tests/install_deploy_test.sh | 71 ++++++++++++++++++++++++++++++++++++
#  2 files changed, 109 insertions(+), 1 deletion(-)
```

### Notas

- No se tocó `tasks.md` porque el fix es una corrección post-verify de T4/T5, no una tarea nueva.
- No se tocó `verify-report.md` (pertenece a la rama docs/PR4 y está untracked acá).

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
| CRITICAL-1 fix | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ 27/27 tests | ✅ New failing tests written | ✅ All pass | ✅ extract fallback, no-source error, persistent-path preference | ✅ Bash 3.2-safe; setUp/tearDown isolation |

### Test Summary

- **Total tests written**: 27 (in `install_deploy_test.sh`)
- **Total tests passing**: 27 / 27
- **Layers used**: Unit (27)
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
**Scope:** T4 + T5 only — deploy pipeline core + service registration + verification + summary + tests + CRITICAL-1 fix.
**Stacked-to-main:** Branch apunta a `main` (PR1 y PR2 ya mergeados en base).
**Size:** ~840 changed lines originales + 109 líneas del fix en `install.sh`/`install_deploy_test.sh`. Exceeds the 400-line budget; `size:exception` fue aprobado porque el deploy pipeline y su matriz de tests forman una unidad cohesiva — un split honesto dejaría un PR intermedio sin el cableado SHA256-a-servicio.

---

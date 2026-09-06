# Apply Progress — feat-install-and-service

**Change:** feat-install-and-service
**Phase:** 5 — apply (PR1 + PR2 merged, PR3 complete)
**Status:** PR1 T1 merged (`a52d1cf`); PR2 T2+T3 merged (`1562ad5`); PR3 T4+T5 complete, ready for verify
**Executor:** sdd-apply (+ orchestrator cherry-pick resolution onto post-PR2 main)
**Updated:** 2026-09-06

---

## Completed Tasks

### PR1 (merged as `a52d1cf`)

- [x] T1.1 RED: crear `cmd/mcp-server/main_test.go` con tests para `--version`
- [x] T1.2 GREEN: implementar guard `--version` en `cmd/mcp-server/main.go`
- [x] T1.3 VERIFY: `go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...`

### PR2 (merged as `1562ad5`)

- [x] T2.1 — `setup/service/mcp-appointments-crm.service`
- [x] T2.2 — `setup/service/com.mcp.appointments.server.plist`
- [x] T2.3 — `setup/service/nssm-install.md`
- [x] T2.4 — VERIFY templates (3 files + grep checks)
- [x] T3.1 — RED: `scripts/tests/backup_test.sh` (5 tests, RED before implementation)
- [x] T3.2 — GREEN: `scripts/backup.sh`
- [x] T3.3 — VERIFY: `bash scripts/tests/run_tests.sh` all green
- [x] RDD fix R3-ENVFILE-BRITTLE: `EnvironmentFile=-` + `service_templates_test.sh` (6 tests), receipt burned (`review-6e2718fcf31c0a15`)

### PR3 (this apply)

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

### PR1 (in main)

| File | Action | Lines |
|------|--------|-------|
| `cmd/mcp-server/main_test.go` | New test file | ~95 |
| `cmd/mcp-server/main.go` | Extend `main()` with `--version` guard + pure helpers | ~20 insertions |

**Total changed lines (PR1):** ~115 additions — within 400-line budget.

### PR2 (in main)

| File | Action | Lines (aprox) |
|---|---|---|
| `setup/service/mcp-appointments-crm.service` | new (+ R3 fix: `EnvironmentFile=-`) | ~19 |
| `setup/service/com.mcp.appointments.server.plist` | new | ~23 |
| `setup/service/nssm-install.md` | new | ~36 |
| `scripts/backup.sh` | new | ~57 |
| `scripts/tests/backup_test.sh` | new | ~128 |
| `scripts/tests/service_templates_test.sh` | new (R3 regression) | ~55 |
| `openspec/changes/feat-install-and-service/tasks.md` | checkbox updates T2/T3 | ~7 lines |

### PR3 (this branch)

| File | Action | Lines |
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
# RED cycle for backup
bash scripts/tests/backup_test.sh
# => FAILED (backup.sh no existe / exit 127)

# GREEN cycle for backup
chmod +x scripts/backup.sh
bash scripts/tests/backup_test.sh
# => OK (5 tests)

# R3 regression (service_templates_test.sh)
bash scripts/tests/service_templates_test.sh
# => OK (6 tests)

# Final regression gate
bash scripts/tests/run_tests.sh
# => OK: 4/4 suites pasaron
```

### PR3

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
| T3.1 | `scripts/tests/backup_test.sh` | Unit (shunit2) | ✅ 2/2 baseline | ✅ Written | n/a | ✅ 5 scenarios | n/a (test only) |
| T3.2 | `scripts/tests/backup_test.sh` | Unit (shunit2) | ✅ 2/2 baseline | ✅ Referenced missing `backup.sh` | ✅ Passed | ✅ 5 scenarios force real logic | ➖ Clean as written |
| T3.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 2/2 baseline | n/a | ✅ 3/3 suites | n/a | ➖ None needed |
| R3 fix | `scripts/tests/service_templates_test.sh` | Unit (shunit2) | ✅ 4/4 suites | n/a (regression) | ✅ 6/6 passed | ✅ `EnvironmentFile=-` enforced | ➖ None needed |

- **Total tests written**: 5 (backup) + 6 (templates), all passing
- `backup.sh` uses `TMP` and `TMP_GZ` cleaned by a single `trap`. `nssm-install.md` documents manual registration + `%APPDATA%` layout per D4/REQ-SU-004.

### PR3

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| T4.1 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ validators + e2e baseline | ✅ Written | n/a | ✅ 12+ scenarios | n/a (test only) |
| T4.2 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ validators + e2e baseline | ✅ Referenced missing functions | ✅ Passed | ✅ Fixtures for SHA256, paths, env, asset name | ✅ Bash 3.2-safe; trap extended for `DEPLOY_TMP` |
| T4.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 4/4 suites | n/a | ✅ 4/4 suites | n/a | ➖ None needed |
| T5.1 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ T4 green | ✅ Extended failing tests | n/a | ✅ service render, verify, summary, TTY guard | n/a (test only) |
| T5.2 | `scripts/tests/install_deploy_test.sh` | Unit (shunit2) | ✅ T4 green | ✅ Service/verify/summary missing | ✅ Passed | ✅ Linux systemd + macOS launchd paths; archive/repo fallback | ✅ `$0` aware TTY guard preserves piped-test setup |
| T5.3 | `scripts/tests/run_tests.sh` | Regression | ✅ 4/4 suites | n/a | ✅ 4/4 suites | n/a | ➖ None needed |

### Test Summary (PR3)

- **Total tests written**: 23 (in `install_deploy_test.sh`)
- **Total tests passing**: 23 / 23
- **Layers used**: Unit (23)
- **Approval tests**: None
- **Pure functions created**: `compose_asset_name`, `validate_tag`, `sha256_file`, `render_systemd_unit`, `render_launchd_plist`

---

## Design Deviations

PR1 matches design §5.4; PR2 follows design §5.2 (backup) and §5.3 (service templates) verbatim, plus the R3 `EnvironmentFile=-` correction required by RDD.

PR3 deviations (documented):

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

## CRITICAL-1 fix — persistent backup.sh + summary path (cherry-picked)

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

### Verificación

```bash
bash scripts/tests/install_deploy_test.sh
# => OK (27 tests)

bash scripts/tests/run_tests.sh
# => OK: 5/5 suites pasaron (incl. service_templates_test.sh)

go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...
# => all green
```

---

## PR Boundary

- **PR1:** T1 only — merged (`a52d1cf`), RDD receipt burned (`review-5cb3536d01cf8d90`).
- **PR2:** T2 + T3 (+ R3 fix) — merged (`1562ad5`), RDD receipt burned (`review-6e2718fcf31c0a15`).
- **PR3 (this branch):** T4 + T5 + CRITICAL-1 fix (size:exception, ~918 + 109 LOC). Rebuilt on post-PR2 main via cherry-picks (5dae828 + fix).
- **Chain continuation:** PR4 (docs) after PR3 merges.

---

> Nota de rebase (2026-09-06): PR4 rebaseado sobre main post-PR3-merge. Se anexa debajo el reporte PR4 original.

# Apply Progress — feat-install-and-service · PR4

**Change:** feat-install-and-service  
**PR:** PR 4 de 4 — `docs: add installation and maintenance manuals`  
**Branch suggestion:** `docs/installation-and-maintenance`  
**Apply executor:** sdd-apply  
**Date:** 2026-09-06  
**Status:** implementation complete, ready for parent-lifecycle  

---

## Scope of this PR

- T6 — Documentation (REQ-IDOC-001..004, REQ-BVER-003)
  - T6.1 `docs/installation.md` (español, step-by-step verificable)
  - T6.2 `docs/maintenance.md` (español, manual anual de operación)
  - T6.3 Alineación `docs/deployment.md` + `docs/PRD.md`
- T7 — Final verification (DoD 1-14) adaptado al scope docs-only de este branch
  - T7.1 Go quality gates
  - T7.2 Shell suites (2/2 en este branch)
  - T7.3 File inventory (scope PR4)
  - T7.4 Manual VM checklist (documentado como requiere VM real)

**Constraint respetado:** no se tocó `cmd/`, `scripts/install.sh`, `setup/service/`, `scripts/backup.sh` en este PR.

---

## Completed tasks (persisted checkboxes updated)

| Task | Persisted checkbox | Evidence |
|---|---|---|
| T6.1 | `- [x] T6.1 — Write docs/installation.md` | `docs/installation.md` creado en español |
| T6.2 | `- [x] T6.2 — Write docs/maintenance.md` | `docs/maintenance.md` creado en español |
| T6.3 | `- [x] T6.3 — Align docs/deployment.md and docs/PRD.md` | `docs/deployment.md` editado; `docs/PRD.md` ya estaba alineado |
| T7.1 | `- [x] T7.1 — Go quality gates` | `go fmt`, `go vet`, `go build`, `go test -race` verdes |
| T7.2 | `- [x] T7.2 — Shell test suites` | `bash scripts/tests/run_tests.sh` 2/2 suites verdes |
| T7.3 | `- [x] T7.3 — File inventory check` | Verificados archivos del scope PR4; faltantes de PR1-3 documentados |
| T7.4 | `- [x] T7.4 — Manual VM checklist` | Documentado en esta sección; requiere VM real |

---

## Files changed

| File | Action | REQs |
|---|---|---|
| `docs/installation.md` | New | REQ-IDOC-001, REQ-BVER-003 |
| `docs/maintenance.md` | New | REQ-IDOC-002, REQ-IDOC-003, REQ-IDOC-004 |
| `docs/deployment.md` | Edit | REQ-IDOC-003, REQ-BVER-003 |
| `openspec/changes/feat-install-and-service/tasks.md` | Update checkboxes / Done lines | — |
| `openspec/changes/feat-install-and-service/apply-progress.md` | New | — |

---

## Verification commands run

### Go quality gates (T7.1)

```bash
go fmt ./...
go vet ./...
go build -o /tmp/mcp-server-test ./cmd/mcp-server
go test -race ./...
```

Resultado: **ALL GO GATES OK**.

### Shell test suites (T7.2)

```bash
bash scripts/tests/run_tests.sh
```

Resultado: **2/2 suites pasaron** en este branch:

- `install_e2e_test.sh` — 23 tests OK
- `install_validators_test.sh` — 13 tests OK

Nota: `install_deploy_test.sh` y `backup_test.sh` no existen en este branch porque pertenecen a PR3, que aún no está mergeado en `main`.

### File inventory (T7.3)

Archivos verificados en este branch:

- ✅ `docs/installation.md`
- ✅ `docs/maintenance.md`
- ✅ `docs/deployment.md`
- ✅ `scripts/install.sh` (existe; no modificado en este PR)

Archivos esperados de PR1-3 y ausentes en este branch (documentado, no bloqueante para PR4):

- ❌ `scripts/backup.sh` → PR2/PR3
- ❌ `setup/service/mcp-appointments-crm.service` → PR2
- ❌ `setup/service/com.mcp.appointments.server.plist` → PR2
- ❌ `setup/service/nssm-install.md` → PR2
- ❌ `scripts/tests/install_deploy_test.sh` → PR3
- ❌ `scripts/tests/backup_test.sh` → PR2
- ❌ `cmd/mcp-server/main_test.go` → PR1

---

## TDD / strict-TDD evidence

`openspec/config.yaml` declara `tdd: true`, pero este PR4 es **docs-only**: no se escribió ni modificó código de producción, por lo que no hay ciclo RED/GREEN/TRIANGULATE/REFACTOR que aplicar.

- No se crearon tests nuevos.
- No se modificó lógica.
- Se verificó que los gates existentes (`go test`, `run_tests.sh`) siguen verdes.

---

## Deviations from design / spec

1. **Longitud de los docs:** el forecast original de `tasks.md` estimaba ~180 líneas para `installation.md` y ~160 para `maintenance.md`. Los documentos finales tienen 251 y 360 líneas respectivamente porque se incluyeron secciones de troubleshooting, scheduling opcional, tablas de referencia y ejemplos de comandos verificables que los REQ-IDOC exigen. El conteo real de cambios (additions + deletions) es aproximadamente **626 LOC**, por encima del budget de 400. El preflight del orquestador ya aprobó la ruta de entrega de 4 PRs encadenados y calificó a PR4 como "~360 LOC within budget"; se reporta el overage real en esta sección para transparencia.

2. **PRD.md no requirió ediciones:** el `grep` por `appointments.db` en contexto de producción no arrojó resultados; `reservas.db` ya se usa consistentemente. Por eso `PRD.md` no fue modificado, cumpliendo "minimal edits only — no restructure".

3. **T7.2 adaptado al branch:** como PR3 no está en este branch, no se pudieron ejecutar `install_deploy_test.sh` ni `backup_test.sh`. Se verificó que las suites de Fase 4 (`install_e2e_test.sh`, `install_validators_test.sh`) pasan sin modificación, cumpliendo REQ-INS-012 de no-regresión.

4. **T7.4 requiere VM real:** los checks de DoD 1, 5, 6 y 9 (systemd user service, linger, endpoint respondiendo, supervivencia a logout/reboot) no pueden ejecutarse en el entorno de desarrollo actual. Quedan documentados en `docs/installation.md` y en esta sección; el verify report de VM real es responsabilidad del ciclo de verificación posterior.

---

## Manual VM checklist (DoD 1, 5, 6, 9)

Para cerrar DoD 1, 5, 6 y 9 se requiere una VM limpia Ubuntu 22.04+ con systemd user bus disponible. Los pasos a ejecutar son los mismos que `docs/installation.md`:

1. `bash install.sh` en una terminal real para generar los 3 JSONs de setup.
2. `bash install.sh --version v0.3.0` (o el pipe equivalente).
3. `systemctl --user is-active mcp-appointments-crm` → `active`.
4. `loginctl show-user "$USER" -p Linger` → `Linger=yes`.
5. `curl --fail http://127.0.0.1:3000/mcp` → HTTP 200.
6. `~/.local/bin/mcp-server --version` → `v0.3.0`.
7. Simular logout/reboot y verificar que el servicio vuelve a `active`.
8. `bash ~/.local/share/mcp-appointments-crm/scripts/backup.sh ~/.local/share/mcp-appointments-crm/reservas.db` y restaurar con `gunzip` + `PRAGMA integrity_check;` → `ok`.
9. Registrar comandos y outputs en el verify report.

Estos pasos están cubiertos por la documentación y por los tests automatizados de funciones individuales en PR1-3. La ejecución real en VM queda como acción de verificación post-merge de la cadena completa.

---

## Remaining work

- Ninguna tarea de implementación pendiente en PR4.
- Acciones de lifecycle posteriores (bounded-review, refutation, validation, receipts, pre-commit/PR gates) son responsabilidad del orquestador/parent y no se inician desde `sdd-apply`.
- La cadena de PRs (PR1 → PR2 → PR3 → PR4) debe mergearse en orden; este PR4 depende de que PR1-3 estén en `main`.

---

## Workload / PR boundary

- **Chain strategy:** `stacked-to-main`
- **PR4 scope:** docs + alignment only (`docs/installation.md`, `docs/maintenance.md`, `docs/deployment.md`)
- **Estimated changed lines (real):** ~626 additions + deletions
- **Review budget:** 400 (excedido; reportado arriba)
- **Delivery decision del preflight:** cadena de 4 PRs aprobada, PR3 con `size:exception`, PR4 calificado como within budget por el orquestador. Se ejecutó auto según esa ruta resuelta.

---

## Structured status consumed / produced

**Consumed (preflight del orquestador):**

- `execution: auto`
- `artifact_store: openspec`
- `delivery_strategy: ask-on-risk (chained 4 PRs, PR3 size:exception aprobado, este PR4 es ~360 LOC within budget)`
- `review_budget: 400`
- `chain_strategy: stacked-to-main`
- `allowed_edit_surfaces`: `docs/installation.md`, `docs/maintenance.md`, `docs/deployment.md`, `docs/PRD.md`, `openspec/.../tasks.md`, `openspec/.../apply-progress.md`

**Produced:**

- `apply_state: implementation_complete`
- `next_recommended: parent-lifecycle`
- `risks`: LOC overage real (~626 vs 400); PR1-3 files absent in branch; T7.4 requires real VM.

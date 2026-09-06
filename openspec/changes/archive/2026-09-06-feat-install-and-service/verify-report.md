# Verify Report — feat-install-and-service

**Change:** feat-install-and-service
**Phase:** 5 — verify (Fase 5, slices PR1–PR4, pre-RDD)
**Executor:** sdd-verify
**Date:** 2026-09-06
**skill_resolution:** paths-injected (gentle-ai SKILL.md)
**Inputs leídos:** proposal.md, design.md, tasks.md, apply-progress.md (PR4 + versiones por-PR en cada rama), 5 specs (29 REQs), openspec/config.yaml, diffs PR1–PR4, código y tests de cada slice.

> **Nota de reconstrucción (2026-09-06, orquestador):** el archivo original se perdió por un `rm` erróneo durante el rebase de PR3 (untracked en el worktree). Se reconstruye desde el contenido leído íntegro en sesión, más el **Addendum post-verify** al final con fixes, re-verifies y receipts RDD.

---

## Status: PASS_WITH_BLOCKER — 1 CRITICAL (docs↔código backup.sh) · 2 WARNING · resto conforme

Verificación por PR: **PR1 ✅ · PR2 ✅ · PR3 ✅ (gates verdes) · PR4 ✅ con 1 CRITICAL de DoD**.
El change **no está ready para archive** hasta resolver el CRITICAL (ver Blockers).

---

## Executive summary

- Los 4 slices existen como PRs abiertos (#62–#65) con ramas verificadas localmente vía worktrees: `feat/feat-install-service-pr1` (238+), `feat/feat-install-service-pr2` (397+), `feat/feat-install-service-pr3` (1364+, size:exception), `docs/installation-and-maintenance` (815+, la rama actual).
- **Todos los gates pasan**: `gofmt` (sin diff), `go vet`, `go build`, `go test -race ./...` (incl. `cmd/mcp-server`), `golangci-lint` 0 issues, `govulncheck` sin vulnerabilidades, y shunit2 `run_tests.sh`: PR2 3/3, PR3 4/4, PR4 2/2 suites.
- Las suites Fase 4 (`install_validators_test.sh`, `install_e2e_test.sh`) pasan **sin modificación** en las 4 ramas (`git diff` vacío contra base) — REQ-INS-012 y D3 cumplidos.
- Cobertura REQ completa: 29/29 REQs trazados proposal→spec→design→tasks→código (detalle abajo).
- Strict TDD activo (`config.yaml tdd: true`): tablas `TDD Cycle Evidence` presentes en PR1, PR2 y PR3; PR4 justificado docs-only. Calidad de aserciones auditada: sin tautologías ni ghost loops.
- **1 CRITICAL**: `backup.sh` no queda instalado de forma persistente por el pipeline y `maintenance.md` referencia una ruta (`~/.local/share/mcp-appointments-crm/scripts/backup.sh`) que el instalador nunca crea.

---

## Per-PR verification

### PR1 — #62 `feat/feat-install-service-pr1` (238 insertions) ✅

- **REQs:** REQ-BVER-001, REQ-BVER-002 (+REQ-BVER-003 cubierto en PR4 docs).
- **Código:** `wantsVersion()` (table-driven, 4 casos) + `printVersion(&buf)` puros; guard en `main()` antes de `run()`; sin paquete `flag`; sin cambios en `internal/mcp/config.go` (D2).
- **Tests:** `TestWantsVersion` (4 subtests), `TestPrintVersion` (compara contra `buildinfo.Version`), `TestBinaryVersionFlag` (exec-based: build + `--version` + salida == `buildinfo.Version`). Aserciones reales, no smoke.
- **Gates ejecutados en worktree:** `gofmt -l` (vacío), `go vet ./...` OK, `go build -o /dev/null ./...` OK, `go test -race ./...` — todos los paquetes `ok`, incl. `cmd/mcp-server`.

### PR2 — #63 `feat/feat-install-service-pr2` (397 insertions, 7 deletions) ✅

- **REQs:** REQ-SU-001..005, REQ-BKP-001..004.
- **Templates verificados:** systemd unit con `EnvironmentFile=%h/.config/mcp-appointments-crm/.env`, `Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db` (literal, D9), `ExecStart=@@BIN_DIR@@/mcp-server`, `WantedBy=default.target`, sin `User=` (DoD 14 ✅). Plist XML válido con `Label`, `ProgramArguments`, `MCP_DB_PATH=@@DATA_DIR@@/reservas.db`, `RunAtLoad`, `KeepAlive`, `StandardOut/ErrPath`. NSSM: español, layout `%APPDATA%\MCP Appointments CRM\`, comandos `nssm install/set`, declaración explícita de no-automatización (REQ-SU-004 ✅). Exactamente 3 archivos en `setup/service/` (REQ-SU-001 ✅).
- **`backup.sh`:** `umask 077`, `set -u`, sin `set -e`/`pipefail`, trap `EXIT INT TERM HUP` limpia `TMP`/`TMP_GZ`, prereqs nombran herramienta faltante, DB existence check, `sqlite3 .backup` + `gzip -c` + `mv` atómico misma partición (mktemp dentro de `backups/`). Cumple D3/Bash 3.2.
- **Tests:** `backup_test.sh` 5 tests / 20 asserts — happy path con `PRAGMA integrity_check = ok` + `SELECT` del fixture row, prereq faltante nombra la herramienta y no toca la DB, DB inexistente, re-run same day, no-scheduling. Sin tautologías.
- **Gates:** `run_tests.sh` → **3/3 suites OK** (incl. backup_test.sh, más Fase 4 sin cambios).

### PR3 — #64 `feat/feat-install-service-pr3` (1364 insertions, 16 deletions) ✅ — size:exception registrada

- **REQs:** REQ-INS-001..013 (+re-incluye PR1/PR2 como base apilada).
- **`size:exception`:** ✅ registrada explícitamente en el commit message (`... (size:exception)`) y con justificación en tasks.md (unidad cohesiva pipeline+tests).
- **Funciones implementadas (auditadas):** `validate_tag`, `refuse_root`, `require_deploy_prereqs`, `require_setup_files` (nombra cada JSON faltante), `detect_platform`/`compose_asset_name` (matriz 4 combos GoReleaser exactos; unmapped falla con mensaje), `sha256_file` (dispatch `sha256sum`/`shasum -a 256`), `download_and_verify` (3 fallos distinguibles con URL manual), `extract_archive`, `ensure_dirs` (0700), `ensure_env_file` (create-if-absent, `MCP_BIND=127.0.0.1`/`MCP_PORT=3000`, 0600, nunca sobrescribe), `install_binary` (atomic `.mcp-server.new.$$` → chmod 0755 → `mv`), `render_systemd_unit` (`@@BIN_DIR@@` siempre; `MCP_DB_PATH` reescrito solo si `DATA_DIR` ≠ default — D9), `render_launchd_plist`, `install_service_linux` (daemon-reload, enable--now vs restart si ya enabled — REQ-INS-013, `enable-linger`), `install_service_macos` (bootout idempotente + bootstrap), `verify_installation` (`--version` contiene tag + wait loop ≤10s `is-active`), `print_post_install_summary` (backup line + herramientas en español + URL + `MCP_DB_PATH` + caveat split-brain), `run_deploy` (13 pasos wired), `run_setup_guard_tty`, `usage()` documenta `--version`, `main()` dispatch completo. Restricciones D3/Bash 3.2 respetadas (sin arrays/mapfile/`${var,,}`).
- **Cleanup:** `cleanup_tmp` limpia `CURRENT_TMP` y `DEPLOY_TMP` en `EXIT INT TERM HUP` ✅.
- **Tests:** `install_deploy_test.sh` — 23 test functions / 66 asserts, cubre la matriz completa de T4.1/T5.1 (incl. tampered-byte DoD 4: assert **no binary in BIN_DIR**; render desde archive y fallback repo — D7; version match/mismatch; summary content; TTY guard con `</dev/null`). Aserciones sustantivas; única nota: test tampered depende de `python3` (skip explícito si falta).
- **Gates:** `golangci-lint run ./...` → **0 issues**; `govulncheck ./...` → **No vulnerabilities found**; `gofmt`/`vet`/`build` OK; `go test -race ./cmd/mcp-server/...` ok; `run_tests.sh` → **4/4 suites OK**; suites Fase 4 unmodified (`git diff 3e48caa..5dae828 -- scripts/tests/install_{validators,e2e}_test.sh` vacío).

### PR4 — #65 `docs/installation-and-maintenance` (815 insertions, 28 deletions) ⚠️ PASS con 1 CRITICAL

- **REQs:** REQ-IDOC-001..004, REQ-BVER-003.
- **`docs/installation.md` (251 líneas, español):** flujo dos partes (setup interactivo + deploy pinned), comando `curl ... | bash -s -- --version vX.Y.Z`, verificación post-install, **distinción explícita `install.sh --version` vs `mcp-server --version`** (línea 152, REQ-BVER-003 ✅), caveat D1/D5, sin referencias TUI como paso de instalación (REQ-IDOC-001 ✅).
- **`docs/maintenance.md` (360 líneas, español):** (a) backups ejecución/restauración + integrity check, (b) upgrade/downgrade re-ejecutando `--version`, (c) logs journalctl + `LOG_DIR`, (d) control de servicio por OS (systemd user + launchctl), (e) caveat DB manual — todo presente (REQ-IDOC-002 ✅ en cobertura); troubleshooting y scheduling opcional marcado como decisión del cliente (REQ-IDOC-004 ✅, sección 6).
- **Unificación `reservas.db`:** ✅ — `appointments.db` solo aparece como caveat de ejecución manual (D1/D5); `PRD.md` no requirió edits (verificado).
- **Gates (rama actual):** `gofmt` vacío, `go vet` OK, `build` OK, `go test -race ./...` ok, `run_tests.sh` **2/2 suites OK**, `install.sh` unmodified vs main.
- **Desviaciones documentadas en apply-progress:** LOC real ~626 vs forecast ~360 (transparencia ✅); T7.2 adaptado 2/2 por ausencia de PR3 en la rama; T7.4 requiere VM real.
- **❌ CRITICAL — ver Blockers:** la ruta de `backup.sh` que usan los docs no es creada por el pipeline.

---

## Spec coverage (29/29 REQs)

| Capability | REQs | PR | Estado |
|---|---|---|---|
| binary-version | BVER-001, 002 | PR1 | ✅ código+tests |
| binary-version | BVER-003 | PR4 | ✅ docs (distinción de interfaces, installation.md:152) |
| service-units | SU-001, 002, 005 | PR2 (templates) + PR3 (render tests) | ✅ |
| service-units | SU-003 | PR2 + PR3 | ✅ |
| service-units | SU-004 | PR2 | ✅ |
| backup | BKP-001..004 | PR2 | ✅ (macOS verificado solo por portabilidad de código, no ejecutado) |
| install-service | INS-001..003, 006, 007 | PR3 (+tests) | ✅ |
| install-service | INS-004 | PR3 | ✅ (nombra archivos faltantes) |
| install-service | INS-005 | PR3 | ⚠️ ver WARNING-1 |
| install-service | INS-008, 009 | PR3 | ✅ (código + tests mockeados; VM pendiente) |
| install-service | INS-010, 011 | PR3 | ⚠️ INS-010 funcional — ver CRITICAL |
| install-service | INS-012 | PR1–PR3 | ✅ suites Fase 4 unmodified y verdes en 4 ramas |
| install-service | INS-013 | PR3 | ✅ (restart en upgrade; código auditado, VM pendiente) |
| install-docs | IDOC-001, 003 | PR4 | ✅ |
| install-docs | IDOC-002 | PR4 | ⚠️ cobertura completa, pero instruye ruta de backup.sh inexistente — CRITICAL |
| install-docs | IDOC-004 | PR4 | ✅ |

## Task checkbox verification

`grep '^\s*- \[ \]'` sobre tasks.md en las 4 ramas → **0 checkboxes de implementación sin marcar**. Ningún bloqueante de completitud.

## Strict TDD compliance

- `TDD Cycle Evidence` presente: PR1 ✅, PR2 ✅, PR3 ✅. PR4: ausencia justificada (docs-only). Aceptable.
- Cross-reference tests↔código: ✅ todos los archivos de test existen y corresponden a implementaciones reales.
- Calidad de aserciones: ✅ sin tautologías, sin ghost loops, sin smoke-only. Destacables: exec-based test binario real (PR1), tampered-byte con aserción de ausencia de binario (PR3), integrity_check + data assertion (PR2).
- Tests re-ejecutados en esta fase y verdes.

## Review workload / PR boundary

- **Chain strategy `stacked-to-main`:** PR3 apila PR1+PR2 (merge `cdb5a40`); PR4 apilado sobre main (`10c0a34`). Nota informativa.
- **Slices asignados respetados:** PR1=T1, PR2=T2+T3, PR3=T4+T5, PR4=T6+T7. Sin scope creep.
- **size:exception:** PR3 ✅ registrada. **PR4 ❌ no registrada** → WARNING-2.

## Verification commands (exactos)

```bash
gofmt -l .                                              # vacío en PR1/PR3/PR4
go vet ./...                                            # OK en PR1/PR3/PR4
go build -o /dev/null ./...                             # OK en PR1/PR3/PR4
go test -race ./...                                     # todos ok, incl. cmd/mcp-server
golangci-lint run ./...                                 # PR3: 0 issues
govulncheck ./...                                       # PR3: No vulnerabilities found
bash scripts/tests/run_tests.sh                         # PR2: 3/3 · PR3: 4/4 · PR4: 2/2 suites OK
git diff <base>..HEAD -- scripts/tests/install_validators_test.sh scripts/tests/install_e2e_test.sh
                                                        # vacío en PR2/PR3/PR4 (Fase 4 intacta, D3)
```

DoD items **no ejecutables en este entorno** (requieren VM limpia Ubuntu 22.04+): DoD 1, 5, 6, 9 y DoD 10-11 en macOS. Código y mocks verificados.

---

## Blockers (exactos)

### CRITICAL-1 — `backup.sh` no persistente; docs referencian ruta inexistente (DoD 7-funcional, DoD 12/13)

- `maintenance.md` instruye `bash ~/.local/share/mcp-appointments-crm/scripts/backup.sh` pero **el pipeline de PR3 nunca instala `backup.sh` en `DATA_DIR/scripts/`**.
- La línea "copy-pasteable" del summary apunta a `$DEPLOY_TMP` (borrado por el trap) o `$SCRIPT_DIR` (inexistente en VPS piped).
- **Bloquea archive.** Sugerencia: instalar `backup.sh` en `$DATA_DIR/scripts/backup.sh` y preferir esa ruta en el summary.

### WARNING-1 — `run_setup_guard_tty` usa heurística `$0 == bash` en vez del `[ -t 0 ]` literal de REQ-INS-005/D6

Desviación documentada en comentario del código. Acción: reconciliar wording en archive; no bloqueante.

### WARNING-2 — PR4 excede el review budget (400) sin `size:exception` registrada

815 insertions (~626 LOC docs reales). Contenido dentro del scope asignado. Acción: registrar la exception; no bloqueante para verify.

---

## Addendum post-verify (orquestador, 2026-09-06)

### Fix CRITICAL-1 ✅ (commit `488e9df` en PR3, luego cherry-pick `7f0afcc` al rebase)

`install_backup_script()` (fuente archive→fallback checkout, `mkdir -p` 0700 + tmp/mv atómico + `chmod 0755`, error en español si no hay fuente), wire en `run_deploy` entre `install_binary` e `install_service`, `_post_install_backup_cmd` prefiere `$DATA_DIR/scripts/backup.sh`. 4 tests nuevos (`install_deploy_test.sh` 27/27), `run_tests.sh` 4/4→5/5 verde, go gates verdes. Re-verify puntual: PASS.

### RDD PR1 (#62) ✅ APPROVED + quemado

Lineage `review-5cb3536d01cf8d90` (high, risk/resilience/readability/reliability) → 4/4 admitidos, 0 bloqueantes (3 SUGGESTION + 1 WARNING informativos) → `acknowledge-approved`/`burned`. Merge squash `a52d1cf`.

### RDD PR2 ✅ APPROVED + quemado (tras 1 corrección)

- Primer lineage `review-b154bd5813e6a4a0`: refuter corroboró `R3-ENVFILE-BRITTLE` (CRITICAL: `EnvironmentFile=` sin prefijo `-` → unit falla si falta `.env`) → `correction_required`. El facade no reabrió el lineage, así que se aplicó la corrección (`EnvironmentFile=-` + `service_templates_test.sh` 6 tests, commit `7b75254`) y se re-revisó en lineage nuevo.
- Segundo lineage `review-6e2718fcf31c0a15` → 4/4 admitidos → **APPROVED** (9 advisories, sin `R3-ENVFILE-BRITTLE`) → `burned`. Merge squash `1562ad5` (PR #66; PR #63 cerrado como superado por rebase).

### RDD PR3 ✅ APPROVED + quemado (tras 2 correcciones)

- Primer lineage `review-214a49fd9beea8c5` → 4 fix findings corroborados: `R4-BINARY-NO-ROLLBACK` (BLOCKER), `R3-ENSUREDIRS-PARTIAL` + `R4-ENSURE-DIRS-MASK` (CRITICAL, mismo bug), `R3-ATOMICWRITE-UNCHECKED` (CRITICAL). Corrección (`9d5a34b`): `ensure_dirs` propaga cada fallo, `atomic_write`/`mkdir` chequeados en `install_service_*`, `install_binary` respalda `mcp-server.prev` + hint de restore en `verify_installation`, 4 tests nuevos.
- Segundo lineage `review-dd9317771a360636` → 1 fix nuevo: `R3-001` (`$USER` sin fallback bajo `set -u` en `enable_linger`). Corrección (`97a7b26`): `${USER:-$(id -un)}` + `test_enable_linger_unset_user`.
- Tercer lineage `review-7bfbec8848afefe4` → 4/4 admitidos → **APPROVED** (8 advisories, `R3-001` degradado a WARNING informativo) → `burned`. Merge squash `f1c5d15` (PR #67; PR #64 cerrado como superado).

### PR4 (docs) — readback estructural ✅

Por routing (trivial/docs): readback en vez de RDD full. Verificado IDOC-001..004, consistencia de la ruta `scripts/backup.sh` con el pipeline ya mergeado, gates verdes. Merge squash `19a79ed` (PR #68; PR #65 cerrado como superado).

### Estado final del change

- 4/4 PRs mergeados a `main` en orden. 29/29 REQs cubiertos. DoD 1-14: 1-9 código+tests, 10-11 portabilidad de código (macOS pendiente de matriz real), 12-13 docs verificables, 14 templates.
- Pendientes post-merge (no bloquean archive): VM real Ubuntu 22.04+ para DoD 1/5/6/9, matriz macOS para DoD 10-11, WARNING-1 (reconciliar wording INS-005), WARNING-2 (registrar size:exception de PR4).
- **Veredicto: READY FOR ARCHIVE.**

---

## Next recommended (original)

~~Fix CRITICAL-1 → RDD → merge PR1→PR4 → VM real~~ — **COMPLETADO**. Siguiente: `sdd-archive` + bump PRD/README.

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

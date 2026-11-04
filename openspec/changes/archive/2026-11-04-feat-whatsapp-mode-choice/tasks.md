# Tasks: feat-whatsapp-mode-choice

> **Change:** feat-whatsapp-mode-choice
> **Status:** doc-first landed (presupuesto de este change; el delivery va docs lane PR)
> **Inputs:** `proposal.md`, `design.md`, `specs/whatsapp-mode-choice/spec.md`,
> `odd/tasks/feat-whatsapp-mode-choice.md`

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (docs de este change) | ~330 (prop 105 + design ~115 + spec ~65 + tasks ~45) |
| 400-line budget risk | Bajo — docs lane, structural readback |
| Chained PRs recommended | No — un solo PR docs (runbook + contract) |

`Decision needed before apply (S3): Yes` (wizard: decisión owner enlista).

## Phase 1 — doc-first (este change)

- [x] TASK-1.1 — proposal + design + spec + tasks con Evidence E1-E8
- [x] TASK-1.2 — matriz D1 / invariantes D2 / criterio por modo D4 alineados a
  los escenarios A/B ya publicados en el contrato (E3-E5) y al smoke modo A
  (E6)
- [x] TASK-1.3 — registro del gap S3 (superficie wizard prenatal, no implementada)

## Phase 2 — delivery (docs lane, un PR)

- [x] TASK-2.1 — **S1**: rama por modo en `docs/gateway-contract.md` §7.5 — bloque **"Mode branch (choose BEFORE step 2)"** con las dos ramas (hermes whatsapp opción 1/2 → pairing → `messenger_id` por modo → C12/C11)_insertado entre el order-constraint y rollback honesty
- [x] TASK-2.2 — **S2**: nueva **§6.1** (matriz de modos + semántica `messenger_id` + invariante mode-free) y §9 extendido con **C11/C12** (spot-check por modo, fuente §6/§6.1)
- [x] TASK-2.3 — PRD: changelog **1.27** (fila tras 1.26, estilo de la casa)
- [x] TASK-2.4 — verificación: readback estructural (tablas + cross-refs + escenarios one-stop), sin Go, GGA sin archivos evaluables — corrida en la entrega 2026-11-01
- **Edit surface:** `docs/gateway-contract.md` (§7.5/§9/nueva sección de
  modos), `docs/PRD.md` (changelog). **Ningún archivo Go.**

## Phase 3 — S3 (wizard oferta del modo; requiere decisión owner)

- [x] **TASK-3.1** — decisión owner (2026-11-04): **NO** — la opción 7 no ofrece la elección del modo; el operador lo resuelve en el TUI de Hermes (`hermes whatsapp`) y este repo aporta la **guía** del procedimiento: `installation.md` § "Configurar el canal WhatsApp (modo A/B)" (regla choose-BEFORE-step-2 en ES + matriz + spot-check C11/C12 + pointer de desconexión). Difiere con disparos de reactivación (primer usuario externo / segunda instalación / productización).
- [x] **TASK-3.2** — ruta "si no" ejecutada sin change de code: fila no-funcional ya registrada en el backlog (con triggers) + la guía del procedimiento como verificación del S3; no encapsuló review nativo alguno (docs lane → readback estructural). Change **cerrado y archivado** `archive/2026-11-04-feat-whatsapp-mode-choice/` (PRD changelog 1.32).

## Coverage check

| Spec requirement | Tasks |
|---|---|
| Runbook por modo | TASK-2.1 |
| Matriz de modos + `messenger_id` | TASK-2.2 |
| Checklist por modo (C-común/modA/modB/gate) | TASK-2.2 |
| persona invariante entre modos | TASK-2.2 (= idempotencia) |
| Gap S3 registrado | TASK-1.3 / TASK-3.1 |

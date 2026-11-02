# Tasks: feat-install-scope-uninstall

> **Change:** feat-install-scope-uninstall
> **Status:** doc-first landed; el código (T1-T4) es entrega posterior y NO
> arranca sin las decisiones OQ del owner.
> **Inputs:** `proposal.md`, `design.md`,
> `specs/install-scope-uninstall/spec.md`, `odd/tasks/feat-install-scope-uninstall.md`

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated (docs de este change) | ~250 (prop 90 + design ~115 + spec ~75 + tasks ~40) |
| Estimated (código futuro) | T2 shell ~250-350 + tests ~200; T3 Go ~120-180 con darkens |
| 400-line budget risk (código) | Medium — T2 (uninstall.sh + tests) probable chained con T3 |
| Chained PRs recommended | Sí si T2+T3 viajan juntos: PR-1 docs+installer scope, PR-2 uninstall.sh+tests, PR-3 RemoveHermesServer Go |

## OQ del owner (RESUELTAS 2026-11-01)

- [x] **OQ-1 — RETIRAR** el zip de Windows de la matriz GoReleaser (covenant Windows = WSL con el asset Linux; `install.sh` hoy no tenía path Windows: la única superficie es la matriz de `.goreleaser.yaml`)
- [x] **OQ-2 — script shell** `scripts/uninstall.sh` (corre aunque el binario no exista)
- [x] **OQ-3 — default PURGA**: el uninstall borra `~/.hermes/skills/recepcionista-negocio/` por default (un Hermes vivo no queda con instrucciones huérfanas apuntando a un server inexistente); flag `--keep-skill` para conservarla — rotación semántica frente al postulado (era default-keep, el owner eligió default-purga)
- [x] **OQ-4 — purgable con warn**: `audit-hash-key` se borra con `--purge-config` + advertencia explícita sobre la auditoría histórica del HMAC
- [x] **OQ-5 — otros agentes = la guía, definición FINAL** (owner 2026-11-01): «Dejemos SOLO A (la guía) como solución definitiva». Ningún adapter/writer por agente (alternativa D descartada) ni el emitir-snippets (B-lite): los agentes fuera de Hermes se conectan con la guía documentada (`docs/installation.md` § "Conectar tu agente MCP", landed `d60c27d` — elementos verificados: OpenClaw / Claude Code / OpenCode + smoke curl); sólo Hermes recibe integración de primera clase (opción 7, cascada, contract de canales). Reabrir sólo si el ecosistema MCP convergiera en un único formato de config (hoy: los formatos fragmentan por cliente y derivan por separado)

## Phase 2 — entrega (después de OQ)

- [ ] **T1 (S1)** — `install.sh`: mensaje por plataforma + hint WSL en el branch
  de plataformas no soportadas (hoy el error genérico); verifier si algún test
  pina ese texto; docs `installation.md` (sección de plataformas) + README +
  Prerrequisitos. **Nota OQ-5: otros agentes NO llevan installer** — la guía
  (`d60c27d`) es la solución definitiva (sin adapters ni B-lite)
- [ ] **T1.5 (candidato, propuesto por el owner 2026-11-02)** — pre-flight del
  puerto en `install.sh`: después de escribir el `.env` y ANTES de levantar el
  unit, un check `ss -tln` sobre `MCP_PORT` (default 3000) que falla con el
  mensaje de las dos palancas (mover el MCP o fijar el `bridge_port`) si el
  puerto está ocupado + test de shell del caso busy. Fundamento: el escenario
  rama A (Hermes-vivo-con-WhatsApp-pareado) entra en crash-loop
  (`Restart=on-failure` 5s, verificado en vivo 2026-11-02 en el journal) — la
  advertencia 3001 de Paso 0/Paso 2 queda desglosada pero el install fallaría
  igual de tarde; el pre-flight lo falla temprano y nombra la salida
- [ ] **T2 (S2/S4)** — `scripts/uninstall.sh` con el contrato D2/D4 completo
  (dry-run default / `--apply` / `--purge-data` doble gate / verificación final
  en tabla) + shell tests: `scripts/tests/uninstall_test.sh` (doble run
  idempotente, keep-data default, live-DB fail-closed, dry-run no-op
  byte-evidence) + golden del dry-run con HOME sandbox; los flags de desacople
  son **Hermes-only** (`--purge-hermes-entry`; skill: default PURGA con
  `--keep-skill`), no per-agent (OQ-5)
- [ ] **T3 (S3)** — `RemoveHermesServer` en `internal/admin/hermes.go` (molde
  `RemoveHermesTelegramOwnerBot`: golden byte-clean, idempotente, entradas
  hermanas sobreviven) + tests admin + decisión de uso del script (reescribe
  el YAML por sí misma — atomic replace — o delega en el binario si está en
  PATH; postulado: script independiente)
- [ ] **T4** — docs del uninstall (sección nueva en installation.md + README)
  y changelog PRD del release que lo traiga
- [ ] **T5 (gates)** — pipeline completo (Go si T3 Go), shellcheck + tests de
  shell de T2, native review por routing (uninstall = riesgo de data), GGA
  por unit

## Coverage check

| Spec requirement | Tasks |
|---|---|
| Plataformas declaradas (Linux/macOS/WSL) | T1 |
| dry-run default + --apply + idempotente | T2 |
| keep-data default + purge explícito con doble gate | T2 |
| Herm desacoplado con flags byte-clean | T2+T3 |
| verificación de ausencia final | T2 |

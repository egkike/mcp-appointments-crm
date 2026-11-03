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

- [x] **T1 (S1)** — `install.sh`: mensaje por plataforma + hint WSL en el branch
  de plataformas no soportadas (hoy el error genérico); verifier si algún test
  pina ese texto; docs `installation.md` (sección de plataformas) + README +
  Prerrequisitos. **Nota OQ-5: otros agentes NO llevan installer** — la guía
  (`d60c27d`) es la solución definitiva (sin adapters ni B-lite)
  **CLOSED 2026-11-03 — issue #163, branch `feat/install-t1-t15-port-preflight`**:
  `_unsupported_platform_error` (1° línea pined de combo preservada + hint WSL2
  para Windows-native/hint de plataformas soportadas para el resto), OQ-1
  ejecutado (`windows` fuera de la matriz GoReleaser: 5→4 assets, zip y
  binario exe template retirados; `goreleaser check` OK); tests
  `test_compose_unsupported_os_wsl_hint` / `test_compose_unsupported_os_generic_list`
  (RED→GREEN) + pin existente intacto; docs: installation.md (intro, Paso 2
  ítems 5/9, Prerrequisitos), README (nota rama A), PRD changelog 1.30, matriz
  PRD §3.5 anotada. **MERGED 2026-11-02: PR #164 squash `e9f5c8d` (CI post-merge run 41544062335 success; pr-gate green: linkage/approval/type + lint/build/test-race/vuln) — issue #163 cerrado por el merge**
- [x] **T1.5 (candidato, propuesto por el owner 2026-11-02)** — pre-flight del
  puerto en `install.sh`: después de escribir el `.env` y ANTES de levantar el
  unit, un check `ss -tln` sobre `MCP_PORT` (default 3000) que falla con el
  mensaje de las dos palancas (mover el MCP o fijar el `bridge_port`) si el
  puerto está ocupado + test de shell del caso busy. Fundamento: el escenario
  rama A (Hermes-vivo-con-WhatsApp-pareado) entra en crash-loop
  (`Restart=on-failure` 5s, verificado en vivo 2026-11-02 en el journal) — la
  advertencia 3001 de Paso 0/Paso 2 queda desglosada pero el install fallaría
  igual de tarde; el pre-flight lo falla temprano y nombra la salida
  **CLOSED 2026-11-03 — mismo branch/issue**: `preflight_port_available` entre
  `ensure_env_file` e `install_binary` (run_deploy), puerto efectivo
  `.env > env > 3000`; listener del propio unit (MainPID) o binario reemplazado
  (exe `(deleted)`) cuenta como propio — upgrade en caliente pasa; sin
  `ss`/`lsof` advierte y continúa. Palanca 1 real: `ensure_env_file` persiste
  `MCP_PORT`/`MCP_BIND` exportados en `.env` nuevo (inválidos/`.env` existentes
  byte-idéntico). Tests RED→GREEN: busy/two-levers, free, own-unit, no-tool
  warn+proceed, validez de puerto, precedencia de fuente **(mismo merge e9f5c8d)**
- [x] **T2 (S2/S4)** — `scripts/uninstall.sh` con el contrato D2/D4 completo
  (dry-run default / `--apply` / `--purge-data` doble gate / verificación final
  en tabla) + shell tests: `scripts/tests/uninstall_test.sh` (doble run
  idempotente, keep-data default, live-DB fail-closed, dry-run no-op
  byte-evidence) + golden del dry-run con HOME sandbox; los flags de desacople
  son **Hermes-only** (`--purge-hermes-entry`; skill: default PURGA con
  `--keep-skill`), no per-agent (OQ-5)
  **CLOSED 2026-11-03 — issue #165, branch `feat/uninstall-t2`**:
  `scripts/uninstall.sh` (bash 3.2 floor; espejo inmutable de `resolve_paths()`
  del installer, incluido el quirk `ENV_FILE` siempre en `~/.config`): sin
  flags = dry-run con tabla por artefacto (BORRARÁ/PRESERVARÁ/TOCARÁ/NO EXISTE)
  y **cero escrituras**; `--apply` ejecuta en orden inverso al install
  (stop+unit+daemon-reload → desacople Hermes → binario+`.prev` → logs →
  config → data) y cada paso reporta; `--purge-data` con doble gate (info de
  DB/tamaño/estado de `backups/`, `--yes` + primera confirmación y segunda
  «ÚLTIMA CONFIRMACIÓN» = `BORRAR` cuando stdin es TTY) y chequeo fail-closed
  **antes de cualquier borrado** (MainPID vivo vía `systemctl --user show -p
  MainPID` o `-wal`/`-shm` no vacíos); `$DATA_DIR/backups/` preservado siempre
  e impreso como decisión; `--purge-config` purga el dir entero con la
  advertencia literal del HMAC (en macOS, donde config==data, purga sólo la
  superficie de config para no tocar la DB sin el doble gate);
  `--purge-hermes-entry` remueve `mcp_servers.mcp-appointments` con awk línea a
  línea (atomic replace, sibling y claves ajenas byte-exactas, `mcp_servers:`
  sin hijos se elimina, no-op byte-idéntico si la entrada no existe,
  `HERMES_HOME` respetado, nunca invoca el binario Go); skill default **PURGA**
  (OQ-3) con `--keep-skill`; tabla final (dry-run y `--apply`) con estados
  removido / preservado por decisión / no existía / intacto / PURGADO y
  fail-closed si un removido reaparece; corre con nada instalado (exit 0).
  Tests: 16 en `scripts/tests/uninstall_test.sh` (sandbox `HOME` mktemp, XDG_*
  unset, fake `systemctl`/`launchctl`) — byte-evidence del dry-run (snapshot
  nombres+sha256), golden `scripts/tests/golden/uninstall_dry_run_plan.txt`
  (post-proceso `__HOME__`, fixture Linux con skip explícito en macOS),
  apply/keep-data con hashes, doble run idempotente, doble confirmación,
  live-DB y WAL no vacío fail-closed, purge-config con warning HMAC, Hermes
  byte-clean/idempotente/childless/config-ausente, keep-skill, CLI y WSL sin
  systemd. `run_tests.sh` no requiere cambios: descubre `*_test.sh` (8/8 suites
  verdes). Sin cambios de comportamiento en `install.sh`; T3 (Go
  `RemoveHermesServer`) sigue abierta. Pipeline: `bash -n` = 0, `shellcheck
  scripts/uninstall.sh` = 0 (sin allowlist), `bash
  scripts/tests/uninstall_test.sh` = 0 (16 tests), `bash
  scripts/tests/run_tests.sh` = 0 (8/8), `go build -o /dev/null ./...` = 0,
  `go test -race ./...` = 0
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

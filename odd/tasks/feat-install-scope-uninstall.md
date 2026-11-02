# Feature: Install scope (Linux/macOS/WSL) + uninstall

**Type**: mixed (installer shell + docs + optional Go) — doc-first landed, código pendiente OQ
**Status**: plan aprobado por owner 2026-11-01 (doc-first `f61cc21`-style entregado)
**Origin**: owner 2026-11-01: "revisar el proceso de instalación; dejar Linux + macOS
(todas hoy ya están así), Windows solo WSL; y teníamos pendiente un proceso de Uninstall."

## Specs

- **S1 (plataformas)**: la guía declara soportadas Linux + macOS nativo y Windows
  vía WSL2 (systemd habilitado); camino nativo fallback .exe/Windows = no-soportado
  por el installer (asset para pruebas manuales).
- **S2 (uninstall contráct)**: `scripts/uninstall.sh` — dry-run por defecto,
  `--apply` explíctico, orden de reversión inverso al install, idempotente,
  verificación final (removido/preservado/no existía).
- **S3 (Hermes desacople)**: `<--purge-hermes-entry>` / `--purge-skill` flags opt-in
  con garantía byte-clean (molde `RemoveHermesTelegramOwnerBot`).
- **S4 (puerta destructiva)**: data del negocio preservada por default;
  `--purge-data --yes` con doble gate y fail-closed si hay runtime vivo sobre la DB.

## Tasks

- **T1** (S1) · route code(shell)+docs · installer messaging + WSL hint + docs plataformas
- **T2** (S2/S4) · route code(shell) · `scripts/uninstall.sh` + shell tests + golden
- **T3** (S3) · route code(Go) · `RemoveHermesServer` (o equivalente del script) + tests
- **T4** (docs) · installation/README/PRD changelog
- **T5** (gates) · pipeline + native review por routing (riesgo: data) + GGA

## Log

- **L1** 2026-11-01 — pedido verbatim del owner (arriba). Relevamiento:
  confirmado que Windows ya no tiene path de install (`install.sh:1378`
  solo `Linux|Darwin`; README ya lo dice), que la deuda del uninstall la
  lleva PRD `:1362` desde 2026-09-24 con requisitos (dry-run default,
  preservar backups, verificar ausencia), y que el inventario de artefactos
  creció: hoy también existe `audit-hash-key` (ADR-0021, cripto) y el
  cleanup de una entrada exacta del config Hermes ya tiene molde probado
  (`RemoveHermesTelegramOwnerBot`). Change `feat-install-scope-uninstall`
  landed doc-first. **OQ resueltas por el owner el mismo día:** (1) retirar el
  zip de Windows de la matriz GoReleaser; (2) uninstall como script shell;
  (3) `--purge-skill` pasa a DEFAULT PURGA (borra la skill de Hermes por
  default; flag `--keep-skill` para conservar); (4) `audit-hash-key` purgable
  con `--purge-config` + warn. Cualquier T puede arrancar (T1+T2 primero).
- **L4** 2026-11-01 (tarde) — **decisión de producto: los agentes fuera de Hermes se conectan con la guía documentada, definición FINAL** (owner: «Dejemos SOLO A (la guía) como solución definitiva para no agregar más dependencias»): sin adapters por agente (D) y sin emitir-snippets (B-lite); verificación upstream: los formatos de config fragmentan por cliente (Claude `.mcp.json`, Cursor `.cursor/mcp.json`, VS Code `.vscode/mcp.json`, OpenClaw `mcp.servers` JSON5, Copilot portable) y derivan por separado — el mercado terceriza esa fricción (mcpconfiggen/grimoire/mcp-layer). La guía `docs/installation.md` § Conectar tu agente MCP (`d60c27d`) es la frontera permanente; sólo Hermes recibe integración de primera clase. Se abre como OQ-5 del change y queda en el non-goal del T3 (flags Hermes-only).
- **L5** 2026-11-03 — **T1 + T1.5 aplicados en un slice** (issue #163, branch
  `feat/install-t1-t15-port-preflight`; owner arrancó con «Vamos con T1 + T1.5»).
  RED→GREEN en `scripts/tests/install_deploy_test.sh` (52 tests OK): hints WSL
  del branch no-soportado con la 1° línea de combo pined, pre-flight de puerto
  entre `ensure_env_file` e `install_binary` (puerto efectivo `.env > env > 3000`;
  listener del propio unit MainPID o binario `(deleted)` cuenta como propio;
  sin `ss`/`lsof` advierte y continúa; busy aborta con dos palancas), y
  `ensure_env_file` persiste `MCP_PORT`/`MCP_BIND` exportados en `.env` nuevo
  (1-65535 fail-closed; `.env` existente byte-idéntico). OQ-1 ejecutado en el
  mismo slice: matriz GoReleaser 5→4 (windows/zip/exe template fuera,
  `goreleaser check` OK). Docs: installation.md (Paso 0 rama A, Paso 2 ítems
  5/9, prerrequisitos), README (nota rama A), PRD changelog 1.30, PRD §3.5.
  Pipeline: fmt/vet/build/test -race + shellcheck (solo SC2015 pre-existing) +
  suite shell 7/7. Gate: native review por routing (default modesto), commit
  único del unit; push/PR decisión owner.

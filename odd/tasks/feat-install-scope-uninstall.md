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
- **L6** 2026-11-03 — **Review nativo del slice T1/T1.5: APPROVED + authority
  burned** (`review-5d517689d364fc13`, tier high por shell/process boundary,
  lentes risk+reliability, base `08a9aa1`, commit candidato `deb4d1d`;
  acknowledge `gentle-ai.review-acknowledged/v1` consumido,
  delivery = ordinary-repository-policy). Advisories no bloqueantes
  (informational, no re-open): **R3-001** reliability WARNING
  `scripts/install.sh:1807-1813` (preflight ss awk/grep pipeline — robustez
  del parseo del listener) y **R3-002** reliability WARNING
  `scripts/install.sh:1505-1508` (validación del `MCP_PORT` exportado).
  Quedan logged como follow-ups potenciales de hardening chico, sin apertura
  de corrección para este candidato. GGA r1 PASSED (0 files matched: patrón
  *.go; diff shell/docs).
- **L7** 2026-11-02 (tarde) — **T1+T1.5 MERGEADO y post-merge al día**: PR #164
  squash `e9f5c8d` (ci 41544062335 success; pr-gate linkage/approval pass),
  issue #163 cerrado; branch local borrada tras verificación byte-idéntica de
  los 8 files del squash. Barrida de consistencia post-covenant `5759df3`
  (docs: PRD — binario distribution/§3.5 banner/DoD ×2/D2; deployment —
  overview/matriz de assets/covenant blockquote "executed"/runbook paso 5;
  changelog histórico y sección legacy NSSM intactos por diseño), ci
  `41545271359` success. Siguiente: T2 (uninstall.sh) — data-risk, nueva sesión.
- **L8** 2026-11-02/03 — **T2 (uninstall.sh) COMPLETO: review nativa APPROVED + authority burned** (`review-347cceb09c782b5b`, consumido `9f17d384...`, gates: 4 ciclos del review por las correcciones acotadas — ver trust chain abajo). Branch `feat/uninstall-t2` con 7 commits (2c5c85b feat base + 6b35ab1 backup guidance + 5 commits fix de hallazgos nativos: 8d222dc macOS backups/stop-failed, f3aa12e pgrep fail-closed, cc6c756 pgrep exit-status, 694c6d4 hier parent-anchored + env-file quirk, 0ea9696d XDG containment refuse); pipeline verde full: bash -n, shellcheck 0 findings, suite uninstall 26/26 + run_tests.sh 8/8, go build + go test -race 15/15 OK. Evidencia en el log de cada de las corrections inline.
- **L9** 2026-11-04 — **T3 (RemoveHermesServer Go) aplicado: RED→GREEN**. Branch `feat/install-t3-remove-hermes-server`, commit único del unit. `internal/admin/hermes.go` += `RemoveHermesServer() bool` (~45 líneas + contract comment): borra la entrada completa `mcp_servers.mcp-appointments` (molde S11 `RemoveHermesTelegramOwnerBot`, observable bool por B15 C1); seccion `mcp_servers:` se dropea sólo si la entrada era su último hijo (paridad con el espejo awk de uninstall.sh); entrada de cualquier shape (incluida escalar) es propiedad de este repo y se remueve; byte-clean e idempotente, cero-value nunca materializado. Sin wiring por diseño (D5: uninstall es script; función reusable para TUI futuro). Tests golden nuevos `internal/admin/hermes_server_test.go` (4 tests + helper `serverSeededConfig`): entrada sólo mía con vecinos vivos, sección dropeada al vaciarse, idempotencia byte-idéntica (sección ausente / sección sin entrada / cero-value), entrada escalar removida. Pipeline: go fmt + vet clean, go build OK, go test -race ./internal/admin OK, suite completa `go test ./...` OK, golangci-lint 0 issues. Gate pendiente: review nativo por routing (riesgo data).
- **L10** 2026-11-04 — **Gate del unit T3: review nativa APPROVED + authority burned** (`review-00fc23b17663c48d`, tier medium, lentes risk+reliability, base `34eb8…`→ candidato `9b51334`, 2/2 reviewers admitidos por host relay, acknowledge `gentle-ai.review-acknowledged/v1` consumido sobre revision `f602f1c1...`; delivery = ordinary-repository-policy). Assess previa: riesgo medium (`executable_change`), reviewDue=false por presupuesto pero review corrida por el plan T5 fijado por owner (routing por riesgo data). Con T3, la pieza de código Go del change queda completa; restan **T4 (docs uninstall: installation/README/PRD changelog)** y el cierre/archive del change. Push/PR decisión owner.

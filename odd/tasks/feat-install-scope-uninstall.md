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
  landed doc-first con OQ-1..OQ-4 abiertas para el owner antes del código.

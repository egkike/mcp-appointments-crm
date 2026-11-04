# Proposal: feat-install-scope-uninstall

## Resumen

Dos piezas de higiene del ciclo de vida del producto, doc-first:

- **S1 — Scope de plataforma para la instalación**: `install.sh` queda
  formally a **Linux + macOS**; para Windows el único camino soportado es
  **WSL** (donde `uname` = Linux y el path del installer funciona tal cual).
  Confirmación del estado (2026-11-01): Windows YA no tiene path nativo —
  la pieza es documentar el covenant WSL + decidir qué hace falta con el
  asset zip de Windows que el release publica.
- **S2 — `scripts/uninstall.sh`**: camino de desinstalación controlado que
  revierte lo que `install.sh` siembra (hoy no existe: PRD `:1362` la
  lista como deuda Fase 2+ desde 2026-09-24).

## Problema

1. El docs dice "Windows tiene asset pero no path" pero no dice **qué le
   decimos al usuario de Windows**: la respuesta correcta es WSL, y nadie la
   escribe en ningún lado. Un usuario de Windows hoy queda solo frente a un
   `.zip` que no va a poder instalar en Windows puro.
2. No existe desinstalación: quitar una instalación de prueba es una
   limpieza manual multi-archivo (doc visto en el smoke from-zero de
   v0.6.1). La lista de artefactos **creció** desde esa nota: hoy el install
   también siembra `audit-hash-key` (ADR-0021: keyfile cripto con perms
   600), templates pristine de la skill v1.1.x, scripts de backup dentro del
   data dir, y (vía opción 7) la entrada en el config del gateway Hermes.
   Un uninstall honesto tiene que nombrar cada una.

## Outcome

Un usuario instala en Linux/macOS/WSL con la guía que nombra su plataforma;
puede desinstalar con un solo comando que, por defecto, **no destruye datos**
(el DB del negocio es el artefacto que más queremos proteger) y imprime
exactamente qué se va a tocar antes de tocarlo.

## Scope (S1-S4 — este change los especifica; el código es fase posterior)

- **S1 — Covenant de plataformas** en `install.sh` + docs: Linux y macOS
  soportados nativamente; Windows soportado **vía WSL** (prerequisito:
  systemd user services habilitado en WSL — `systemd=true` en
  `/etc/wsl.conf`, Ubuntu 22.04+); decisión sobre el asset zip Windows:
  se mantiene o se retira de la matriz GoReleaser (OQ-1).
- **S2 — `scripts/uninstall.sh`** con el contrato de PRD (mejorado):
  **`--dry-run` default** (imprime el plan, no toca nada), flags
  `--apply` para ejecutar; reporte exacto por artefacto; verificación de
  ausencia al terminar; preservación por defecto de todos los datos
  (`reservas.db`, `-wal/-shm`, `.env`, `caller-id`, `audit-hash-key`,
  backups); remoción de: service unit (systemd --user o launchd),
  binario, dir de logs, templates pristine, scripts copiados.
- **S3 — Desacople del gateway Hermes** (opción 7 inversa): remover la
  entrada `mcp_servers.mcp-appointments` de `~/.hermes/config.yaml` con la
  misma garantía byte-clean/idempotente del precedent
  (`RemoveHermesTelegramOwnerBot`, S11); el `$--bin` de la skill instalada
  por la cascada (`~/.hermes/skills/recepcionista-negocio/`) es un
  sub-flag (`--purge-skill`, default NO) — artefacto del runtime del
  gateway, no del MCP.
- **S4 — Rama destructiva explicitada**: `--purge-data` como flag separado (doble gate al estilo del purge-inactive del TUI) que borra el DB y sus WAL/shm; los **backups generados** por `backup.sh` siempre se preservan. Sin el flag, la desinstalación no toca datos.

## Non-goals

- No platform changes real au runtime del server: el gate Windows ACL
  (#158) y el HMAC de Windows (#159) quedan intactos — son de la
  plataforma del binario, no del installer; si OQ-1 decide quitar el
  zip, el código sigue compilando y las aprobaciones muentradas siguen.
- No touching de la cascada/instalación de la skill del gateway (la del
  uninstall borra el file si hay flag, no re-instala nada).
- No fuzzing of the JSON setup imports.

## Constraints / fences

- Todo lo que instala el script: `--dry-run` as post-instalación-heatmap
  evidence: uninstall và instal
  formation.
- El uninstall corre sólo con el servicio detenido por el script (lo
  detiene él); nunca contra un servicio activo que SIGUe usando la DB.
- Los paths de datos son los del XDG default del repo
  (`docs/installation.md` layout) — el uninstall respeta `MCP_DB_PATH` /
  `MCP_CONFIG_DIR` del `.env` si están ahí.

## Evidence Inventory

- E1: `scripts/install.sh:1378` — `case` de asset paths con `Linux|Darwin`
  únicamente; `:1692` guardia systemd; `:1577-1652` launchd path completo.
- E2: `.goreleaser.yaml:23-53` — matrix `goos: [linux, darwin, windows]`
  con `binary` de `mcp-server_windows_amd64.exe` (ADR-0014 Decision 1/3).
- E3: `README.md:52` — "Windows has the asset but no install path" +
  ADR-0014 Decision 3 (referida por installation.md `:148`).
- E4: `docs/PRD.md:1362` — la deuda base del uninstall con requisitos
  previos (dry-run default, preservar backups, verificar ausencia).
- E5: Artsfile del колinstall (estado v0.10.0, verificado en el VM):
  binario `~/.local/bin/mcp-server`; data dir `~/.local/share/
  mcp-appointments-crm/` (reservas.db + `-wal/-shm` + scripts/backup.sh +
  backups/); config dir `~/.config/mcp-appointments-crm/` (`.env`,
  `caller-id`, **`audit-hash-key`** nuevo por ADR-0021, `templates/
  recepcionista-negocio/SKILL.md` pristine v1.1.x, setup/); logs dir
  (XDG); service unit systemd --user.
- E6: Opcón 7 escribe y S11 helper: `internal/admin/hermes.go`
  (`RemoveHermesTelegramOwnerBot` byte-clean idempotente) — precedent de
  la remoción exacta de claves del config Hermes runtime-owned.
- E7: cascada instala `~/.hermes/skills/recepcionista-negocio/SKILL.md`
  (verificado en el VM 2026-11-01, v1.1.1 viviendo ahí).

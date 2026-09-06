# Exploration — feat-install-and-service (Fase 5)

> **Change:** feat-install-and-service — Fase 5 install-and-service
> **Date:** 2026-09-06
> **Agent:** sdd-explore (glm-5.3-flash, fallback sin shell/CodeGraph, lectura directa)
> **skill_resolution:** paths-injected (gentle-ai SKILL.md)

## Q1: GitHub Releases asset naming — RESUELTA

**Fuentes:** `docs/deployment.md` (§Artifacts) + `docs/architecture/0014-release-and-deploy-workflow.md` (Decision 1, líneas 70-90). Coinciden exactamente.

- Tag format: `vMAJOR.MINOR.PATCH` semver anotado (`v0.3.0`), pre-releases `vX.Y.Z-rc.N`/`-beta.N` (no `latest`).
- Assets por release (6): `mcp-appointments-crm_Linux_x86_64.tar.gz`, `mcp-appointments-crm_Linux_arm64.tar.gz`, `mcp-appointments-crm_Darwin_x86_64.tar.gz`, `mcp-appointments-crm_Darwin_arm64.tar.gz`, `mcp-appointments-crm_Windows_x86_64.zip`, `checksums.txt` (SHA256 de los 5).
- Binario interno dentro del archive: `mcp-server` / `mcp-server.exe`. GoReleaser compila con `CGO_ENABLED=0` + `ldflags` version. URLs: `https://github.com/egkike/mcp-appointments-crm/releases/download/{tag}/{asset}`
- Maps OS/arch: `uname -s` (Linux/Darwin) + `uname -m` (x86_64/arm64) → nombre GoReleaser (Linux/Darwin/Windows + x86_64/arm64).

## Q2: Estructura actual de scripts/install.sh (Fase 4) — MAPEADA

**1053 líneas.** Header (líneas 1-17): Bash 3.2 floor (sin arrays asociativos, `mapfile`, `declare -n`, `${var,,}`, `pipefail`, `set -e`), `umask 077`, `set -u`, trap `EXIT/INT/TERM/HUP` → `cleanup_tmp`; globals `CURRENT_TMP`, `CONFIG_DIR`, `SETUP_DIR`, `CHECKPOINT_PATH`.

**Secciones:**
- helpers de strings byte a byte (`trim_value`, `is_blank`, `char_code`, `str_toupper`) ~31-170
- validators `v_*` (`nonempty`, `country`, `email`, `phone`, `url`, `lat/lon`, `currency`, `tz`, `hhmm`, `time_pair`, `price`...) 172-232
- transforms `t_*` 233+; JSON helpers `json_escape`/`json_unescape`/`_json_array`/`json_value` 251-430
- `atomic_write` (~510-526): temp file `${dest}.new.$$` + `mv`
- `resolve_paths()` 527-546: Darwin → `$HOME/Library/Application Support/MCP Appointments CRM`; else `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm`; rechaza symlink en `CONFIG_DIR`; setea `SETUP_DIR=$CONFIG_DIR/setup`, `CHECKPOINT_PATH=$CONFIG_DIR/setup.json.tmp`
- prompt engine (`prompt_yes_no`), flujos `run_business`/`hours`/`staff`/`services`/`summary` (555-943)
- `finalize()` 944-997: renderiza `setup_business.json`, `setup_staff.json`, `setup_services.json` con `atomic_write`, valida con `jq`/`python3`, borra checkpoint, conserva checkpoint en error
- `setup_files_exist()`, `prompt_rsq()` (R/S/Q resume), `prompt_reconfigure()`, `run_setup()` 1032-1062
- `usage()` + `main()` 1064-1080: SOLO acepta `--setup-only` y `--help`; argumento desconocido → error.

**Punto de extensión:** agregar en `main()` un modo deploy (ej. `--deploy` o default cuando no es TTY) y extender `resolve_paths()` para `DATA_DIR`/`BIN_DIR`/`LOG_DIR` antes del flujo interactivo. El pipeline download/service va DESPUÉS de `finalize()` en `run_setup`, o como flujo paralelo reutilizando `resolve_paths`.

## Q3: Ubicación SQLite DB relativa al binario — RESUELTA (con conflicto a decidir)

- Código actual (`cmd/mcp-server/main.go:90-95`): `dbPath := os.Getenv("MCP_DB_PATH")`, si vacío → default `./data/appointments.db` (relativo al CWD). No hay path XDG en el binario.
- PRD §3.5 (`docs/PRD.md:134-150`) + `deployment.md`: data debe vivir en `~/.local/share/mcp-appointments-crm/` (Linux, respeta `XDG_DATA_HOME`), `~/Library/Application Support/MCP Appointments CRM/` (macOS), `%APPDATA%\MCP Appointments CRM\` (Windows). Docs usan nombre `reservas.db` para backups (`backups/reservas-YYYYMMDD.db.gz`).
- ⚠️ **CONFLICTO:** binario default `./data/appointments.db` vs layout PRD `~/.local/share/mcp-appointments-crm/reservas.db`. Dos opciones:
  - (a) service unit pasa `Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db` (sin tocar Go)
  - (b) cambiar default del binario a path XDG (cambio Go, mayor riesgo)
  - La (a) es la mínima y consistente con el diseño actual del binario; el nombre `reservas.db` vs `appointments.db` debe normalizarse en spec (recomendado: `reservas.db` vía `MCP_DB_PATH` del service unit, o unificar docs).
- `db.NewDatabase` crea el directorio y corre `initSchema` idempotente (`internal/db/database.go`, DSN con pragmas `WAL`/`busy_timeout` vía `buildDSN`).

## Q4: Flags/env vars de install.sh — RESUELTA

- Actual: solo `--setup-only` (default) y `--help`. No hay `--version`, no lee `MCP_BIND`/`MCP_PORT`.
- `deployment.md` (target): `bash install.sh --version vX.Y.Z` (pinned) — flag `--version` es la interfaz comprometida en docs; `curl|bash` → `bash -s -- --version v0.3.0`.
- Env del binario (`internal/mcp/config.go`, ADR-0007): `MCP_BIND` (default `127.0.0.1`, solo loopback `127.0.0.0/8` o `::1`, `ValidateLoopback` falla al arranque) y `MCP_PORT` (default `3000`, sin auto-fallback). Precedencia: `env vars` > `~/.config/mcp-appointments-crm/.env` (`LoadDotEnv`, tier opcional) > defaults.
- Recomendación spec: `install.sh` acepta `--version`, `--bind`, `--port` (o respeta `MCP_BIND`/`MCP_PORT` env) y escribe `~/.config/mcp-appointments-crm/.env` (crear si no existe, nunca sobrescribir) con `MCP_BIND=127.0.0.1`, `MCP_PORT=3000`.

## Q5: Env vars del service unit — RESUELTA

- Service unit systemd user: `~/.config/systemd/user/mcp-appointments-crm.service`, carga `EnvironmentFile=%h/.config/mcp-appointments-crm/.env` (PRD §3.5, `deployment.md`).
- Vars necesarias: `MCP_BIND=127.0.0.1`, `MCP_PORT=3000` (en `.env`, generado por `install.sh`) + `MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db` (recomendado en el unit o `.env`, ver conflicto Q3). Logs: `journal` (stderr) y/o `~/.local/state/mcp-appointments-crm/mcp-server.log` (`XDG_STATE_HOME`).
- Linger: `install.sh` ejecuta `loginctl enable-linger $USER` (Linux); macOS `launchd` `~/Library/LaunchAgents/com.mcp.appointments.server.plist` persiste por defecto.

## Mapa de codebase

- `scripts/`: `install.sh` (1053 l, único script existente) + `tests/install_validators_test.sh`, `tests/install_e2e_test.sh`, `tests/lib/shunit2`. **NO existe** `backup.sh` (`grep` 0 matches). **NO existe** `install.ps1` aún (`deployment.md` lo referencia pero es target Fase 5+).
- `setup/service/`: **NO EXISTE** — a crear (templates systemd user unit, launchd plist, NSSM placeholder Windows).
- `.goreleaser.yml`: **NO existe**. `.github/workflows`: solo `ci.yml` (jobs en `ubuntu-latest`). `release.yml` es Fase 6 (non-scope, confirmado).
- Binario: `cmd/mcp-server/main.go` — sin flag parsing (NO hay `--version`/`--register-service` implementados; `grep flag./os.Args` en `cmd/` = 0 matches). `buildinfo.Version="dev"` via `ldflags` pero no expuesto como flag CLI → **GAP:** `deployment.md` promete `mcp-server --version`; Fase 5 debe decidir si lo agrega (recomendado: flag mínimo `--version` en `main.go`, ~20 LOC) o lo difiere a Fase 6.
- `internal/mcp/config.go`: `LoadConfig` + `loadConfigFrom(envPath)`, `dotenv.go` en `internal/config`. `internal/db/database.go`: `NewDatabase`, `buildDSN`.
- Docs existentes como fuente de verdad: `docs/deployment.md` (guía completa ya escrita — es el SPEC DE FACTO del instalador: layout XDG, `.env`, linger, checksums, troubleshooting), `docs/PRD.md` §3.5, ADR-0007 (bind/port), ADR-0014 (release/assets), ADR-0001 (no Docker), ADR-0002 (no root), ADR-0005 (backup manual, sin scheduler).

## Riesgos / decisiones para proposal

1. Conflicto DB path (Q3) — decidir `MCP_DB_PATH` en unit vs cambio de default en Go.
2. `mcp-server --version` no existe — dependency de DoD/install UX; agregar flag o ajustar docs.
3. `install.sh` debe seguir Bash 3.2 floor y no romper los tests `shunit2` existentes (extender, no reescribir).
4. macOS data dir difiere de Linux (`Library/Application Support`) — `resolve_paths` ya maneja `CONFIG_DIR`; extender para `DATA_DIR`/`BIN_DIR`/`LOG_DIR` por OS.
5. Nombre de DB (`reservas.db` docs vs `appointments.db` código) — unificar.
6. `curl|bash` no-TTY: flujo interactivo Fase 4 asume `read`; pipeline deploy debe degradar a modo no interactivo (`check [ -t 0 ]`) con mensajes claros.

## Status

Explore completo — listo para `sdd-proposal`. Las 5 preguntas quedan resueltas con evidencia citada arriba.

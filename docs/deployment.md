# Deployment & Release Guide

> **Source of truth for releases**: GitHub Releases. Assets are published by the
> tag-triggered GoReleaser CI workflow (see Release Process).
> Rationale and trade-offs are in [ADR-0014](./architecture/0014-release-and-deploy-workflow.md).

## Overview

`mcp-appointments-crm` ships as a single Go binary (pure Go via `modernc.org/sqlite`,
no CGo, no Docker) for 5 targets. Every `vX.Y.Z` tag publishes 5 archives + a SHA256
`checksums.txt` on GitHub Releases, built by the tag-triggered GoReleaser workflow with no
manual upload. The only historical release assembled by hand is the `v0.3.0` demo, which
ships a single Linux x86_64 archive plus `checksums.txt`; every tag after the pipeline
carries the full matrix. The install script downloads the
correct archive over HTTPS, verifies its checksum, installs to user-level paths, registers a
user-level service, and verifies the installed binary plus the service state. The
installer issues no HTTP request of its own; liveness is `http://127.0.0.1:3000/healthz`
(a bare `GET /mcp` answers **405 by design** — the MCP endpoint accepts POST JSON-RPC only).

- Repo: `https://github.com/egkike/mcp-appointments-crm`
- Install script (raw):
  - Unix: `https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh`
  - Windows: **not implemented** — there is no `scripts/install.ps1`. See the Windows
    section below and [ADR-0014](./architecture/0014-release-and-deploy-workflow.md).
- Default endpoint: `http://127.0.0.1:3000/mcp` (loopback only, see ADR-0007).

## Release Process

### Step-by-step

1. **Develop on a feature branch**, open a PR against `main`.
2. **CI must be green** (`go vet`, `golangci-lint`, `go test -race`, `go build`).
   PR review approval required (see `AGENTS.md` pre-flight pipeline and GGA hook).
3. **Squash and merge** to `main`. `main` is always releasable.
4. **Tag the release** (annotated, semver — see Versioning):

   ```bash
   git checkout main
   git pull origin main
   git tag -a v0.3.0 -m "v0.3.0"
   git push origin v0.3.0
   ```

5. **CI builds and publishes the release** — the tag push triggers
   `.github/workflows/release.yml` (`name: release`, `on: push: tags: ['v*']`,
   `permissions: contents: write`), which runs GoReleaser `v2.18.2` against
   `.goreleaser.yaml`:
   - cross-compile all 5 targets with `CGO_ENABLED=0` (no cross toolchain required)
   - assemble each archive with the required content set: `mcp-server`
     (`mcp-server_windows_amd64.exe` on Windows), `scripts/backup.sh`,
     `setup/service/mcp-appointments-crm.service`, `setup/service/com.mcp.appointments.server.plist`
   - compute `checksums.txt` (SHA256 over all archives, `sha256sum`-compatible)
   - create the GitHub Release and attach every asset — no manual build, no manual upload

The only release assembled by hand is the historical `v0.3.0` demo (release title
"v0.3.0 (demo)"), uploaded through the GitHub web UI before this pipeline existed.

6. **Verify the release** — these are operator commands run by hand, not CI output:

   ```bash
   gh release view v0.3.0 --repo egkike/mcp-appointments-crm
   curl -fsSL https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/checksums.txt | cat
   mcp-server --version   # after installing that tag
   curl --fail http://127.0.0.1:3000/healthz
   ```

If a release is wrong, either delete the remote tag and re-tag, or replace the assets
under the same tag (the first manual upload of `v0.3.0` shipped only the binary and was
replaced by the complete asset about an hour later):

```bash
git push --delete origin v0.3.0
git tag -d v0.3.0
# fix, then re-tag and push
```

> **Local dry-run before tagging (standard procedure).** The pipeline is the shipped
> implementation of the `GoReleaser + releases por CI` backlog item
> ([PRD §7](./PRD.md#7-roadmap-y-fases), Fase N; [ADR-0014](./architecture/0014-release-and-deploy-workflow.md)).
> Run it locally with the pinned GoReleaser (`v2.18.2`) before pushing a tag — no
> `GITHUB_TOKEN` and no network required:
>
> ```bash
> goreleaser check
> goreleaser release --snapshot --clean --skip=publish
> ```
>
> A healthy snapshot run produces the 6 contract files in `dist/` (see Artifacts); the
> snapshot version string looks like `0.3.0-SNAPSHOT-8eae404`.

## Versioning

- **SemVer** `vMAJOR.MINOR.PATCH` (e.g. `v0.3.0`, `v1.0.0`). Tags must match `v*`.
- **Conventional Commits** drive the changelog (`feat:`, `fix:`, `docs:`, etc.).
  Breaking changes use `feat!:` / `fix!:` or `BREAKING CHANGE:` footer and bump MAJOR.
- Pre-releases use `vX.Y.Z-rc.N` / `vX.Y.Z-beta.N` and are published as
  GitHub pre-releases (not `latest`).

> **Two `--version` interfaces exist:**
> - `install.sh --version vX.Y.Z` selects the release to deploy (installer flag).
> - `mcp-server --version` reports the installed binary version (bare version string).
> Do not confuse them; see the verification sections below.

## Artifacts

Each `vX.Y.Z` release publishes 6 files:

> **Produced per tag by the pipeline: all 6 files.** The GoReleaser workflow
> (`.github/workflows/release.yml` + `.goreleaser.yaml`) builds every row below on each
> `vX.Y.Z` tag. The historical `v0.3.0` demo, assembled by hand, remains the only release
> that ships just `mcp-appointments-crm_Linux_x86_64.tar.gz` + `checksums.txt`.

| File | Platform | Arch | Service manager |
|---|---|---|---|
| `mcp-appointments-crm_Linux_x86_64.tar.gz` | linux | amd64 | systemd (`--user`) |
| `mcp-appointments-crm_Linux_arm64.tar.gz` | linux | arm64 | systemd (`--user`) |
| `mcp-appointments-crm_Darwin_x86_64.tar.gz` | darwin | amd64 | launchd |
| `mcp-appointments-crm_Darwin_arm64.tar.gz` | darwin | arm64 | launchd |
| `mcp-appointments-crm_Windows_x86_64.zip` | windows | amd64 | NSSM or Task Scheduler |
| `checksums.txt` | — | — | SHA256 for the 5 archives |

All archives contain the binary `mcp-server` (or `mcp-server_windows_amd64.exe` on Windows),
the service templates (`setup/service/mcp-appointments-crm.service` /
`com.mcp.appointments.server.plist`), and the `scripts/backup.sh` helper — the 4
entries the published `v0.3.0` asset carries. This content set is the contract
`install.sh` relies on: it reads `mcp-server`, `scripts/backup.sh` and the matching
`setup/service/*` template out of the extracted archive. `scripts/install.sh` is
never inside the archive — it is curled from `raw.githubusercontent.com`. An earlier
manual upload of the `v0.3.0` tag shipped only the binary and broke the install; that
asset was replaced about an hour later by the complete one, which is what is published
today.

Version is embedded via `ldflags` into `internal/buildinfo.Version` — the only
build-time variable that exists today (there is no `Commit`/`Date`), set with
`-X github.com/egkike/mcp-appointments-crm/internal/buildinfo.Version={{.Version}}`.
Verify with:

```bash
mcp-server --version
# v0.3.0
```

The binary prints the bare version tag so the installer can match it against the requested release tag.

## Install — Linux

### One-line (recommended)

> **Pinning is mandatory.** The installer resolves no `latest` and rejects pre-releases;
> a piped invocation without `--version` falls through to the interactive wizard, which
> needs a real terminal, so it aborts with
> `Error: el modo interactivo requiere una terminal.` and exit 1.

```bash
# pinned version (the only supported one-line form)
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0

# first-time setup needs the interactive wizard, which cannot be piped —
# run it in a real terminal instead (see docs/installation.md):
#   curl -fsSLO https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh && bash install.sh
```

### HomeLab VM example (Tailscale `100.95.242.72`)

The canonical deployment for this project is the Linux HomeLab VM reachable over
Tailscale. No Go toolchain is required on the VM.

```bash
# from your workstation, over Tailscale
ssh kike@100.95.242.72

# on the VM - run the installer, pinned (the wizard step must run earlier in a real
# terminal; see docs/installation.md)
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0

# check
systemctl --user is-active mcp-appointments-crm
systemctl --user status mcp-appointments-crm --no-pager
curl --fail http://127.0.0.1:3000/healthz
```

### Manual download (no pipe)

```bash
curl -fsSLO https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh
bash install.sh --version v0.3.0
# equivalent to: curl -fsSL …/install.sh | bash -s -- --version v0.3.0
```

### What gets installed (Linux XDG layout)

| Component | Path |
|---|---|
| Binary | `~/.local/bin/mcp-server` (respects `$XDG_DATA_HOME` if set) |
| Data (SQLite + backups) | `~/.local/share/mcp-appointments-crm/` (`reservas.db`, `reservas.db-wal`, `reservas.db-shm`, `backups/`) |
| Config (JSON + `.env`) | `~/.config/mcp-appointments-crm/` (`.env`, `setup/`) |
| Logs | User journal: `journalctl --user -u mcp-appointments-crm` (the systemd unit writes no log file) |
| Service unit | `~/.config/systemd/user/mcp-appointments-crm.service` |

The `.env` file (created if absent, never overwritten) holds loopback config:

```bash
# ~/.config/mcp-appointments-crm/.env
MCP_BIND=127.0.0.1
MCP_PORT=3000
```

The service unit loads it via `EnvironmentFile=%h/.config/mcp-appointments-crm/.env`.
Precedence is: system env vars > `.env` > defaults (`127.0.0.1:3000`) — see ADR-0007.

`install.sh` runs `loginctl enable-linger $USER` so the user service survives logout
(24/7 on a VPS — ADR-0002). Verify:

```bash
loginctl show-user $USER -p Linger  # Linger=yes
```

### Verification (Linux)

```bash
# service
systemctl --user is-active mcp-appointments-crm
systemctl --user status mcp-appointments-crm --no-pager
journalctl --user -u mcp-appointments-crm -n 50 --no-pager

# health — loopback only (liveness endpoint)
curl --fail http://127.0.0.1:3000/healthz   # 200 {"status":"ok",...}
# with custom port
curl --fail http://127.0.0.1:${MCP_PORT:-3000}/healthz
# Note: a bare GET on /mcp answers 405 by design (REQ-MT-002) — the MCP
# endpoint accepts POST JSON-RPC only. It proves the server is up and
# routing, but /healthz is the liveness check.

# database
sqlite3 ~/.local/share/mcp-appointments-crm/reservas.db \
  "SELECT name FROM sqlite_master WHERE type='table';"
sqlite3 ~/.local/share/mcp-appointments-crm/reservas.db \
  "PRAGMA journal_mode; PRAGMA busy_timeout;"

# version
mcp-server --version
~/.local/bin/mcp-server --version
```

## Install — macOS

> **`Darwin` asset disponible desde v0.4.0.** El pipeline de GoReleaser publica
> `mcp-appointments-crm_Darwin_x86_64.tar.gz` y
> `mcp-appointments-crm_Darwin_arm64.tar.gz` junto con `checksums.txt` en cada tag,
> así que `install.sh` resuelve la plataforma con `uname -s`/`uname -m` y descarga
> el archivo correcto. Un build local desde el source sigue siendo el fallback
> ([PRD §7](./PRD.md#7-roadmap-y-fases)).

Mismo camino `curl | bash` que Linux (detecta `Darwin` vía `uname -s`):

```bash
# pinned (required — the installer resolves no `latest` and rejects pre-releases;
# a piped invocation without --version needs a real terminal and aborts)
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.6.1
```

Service registration uses `launchd`:

| Component | Path (macOS) |
|---|---|
| Binary | `~/.local/bin/mcp-server` |
| Data | `~/Library/Application Support/MCP Appointments CRM/` |
| Config | setup JSONs: `~/Library/Application Support/MCP Appointments CRM/setup/`; the `.env` the binary reads: `~/.config/mcp-appointments-crm/.env` |
| Logs | `~/Library/Logs/MCP Appointments CRM/mcp-server.out.log` + `mcp-server.err.log` (launchd `StandardOutPath` / `StandardErrorPath`) |
| Agent plist | `~/Library/LaunchAgents/com.mcp.appointments.server.plist` |

Verification:

```bash
launchctl list | grep com.mcp.appointments
launchctl print gui/$UID/com.mcp.appointments.server
curl --fail http://127.0.0.1:3000/healthz
log show --predicate 'process == "mcp-server"' --last 5m
```

No `loginctl` step on macOS — user LaunchAgents persist after logout by default.

## Install — Windows

> **Not available yet.** There is no supported Windows install path today. Everything
> in this section describes the **target design**, not shipped functionality:
>
> - `scripts/install.ps1` does not exist in the repository.
> - `mcp-server --register-service` is not implemented; the binary only supports
>   `--version`.
> - Sí se publica un asset Windows (`mcp-appointments-crm_Windows_x86_64.zip` desde
>   v0.4.0, junto con los assets de Linux y macOS y `checksums.txt`), pero no existe
>   un path de instalación soportado que lo consuma.
> - There is no Task Scheduler template; `setup/service/` ships the systemd unit, the
>   launchd plist and the manual `nssm-install.md` guide.
>
> Windows install automation was declared a non-goal of Fase 5
> (`openspec/changes/archive/2026-09-06-feat-install-and-service/`, REQ-SU-004) and is
> tracked as pending scope in [docs/PRD.md §7](./PRD.md#7-roadmap-y-fases). For manual
> service registration guidance (untested in CI) see
> [`setup/service/nssm-install.md`](../setup/service/nssm-install.md).

Two paths. **Primary is `go install`** (no SmartScreen "Unknown publisher" dialog,
no cert cost). Fallback is `install.ps1` (prebuilt EXE, shows unsigned warning) —
neither is implemented yet.

### Primary — `go install` (recommended)

Requires Go once (`winget install Go.Go` or `scoop install go`):

```powershell
# NOT IMPLEMENTED — target design only (see the "Not available yet" note above)

# latest
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@latest

# pinned version
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0

# verify
mcp-server --version
# ensure %USERPROFILE%\go\bin is on PATH

# register as user-level service (Task Scheduler; NSSM if available) — NOT IMPLEMENTED
mcp-server --register-service
# or: mcp-server install-service

# health
Invoke-RestMethod http://127.0.0.1:3000/mcp
# NOT IMPLEMENTED — target design only
Get-ScheduledTask -TaskName "mcp-appointments-crm" | Get-ScheduledTaskInfo
```

Why this avoids SmartScreen: the binary is **built locally** (`go` fetches module
sources over HTTPS and compiles). It never arrives as a downloaded EXE, so it
never receives a Mark of the Web (MotW) alternate data stream and never triggers
the SmartScreen publisher-reputation interstitial. No OV/EV certificate required.

### Fallback — `install.ps1` (prebuilt EXE) — not implemented

> The script below does not exist in the repository. The commands are retained as
> target-design reference only; see the section note above.

```powershell
# latest — downloads mcp-server_windows_amd64.exe from GitHub Releases
irm https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1 | iex

# pinned — avoid piping when pinning (parameterized invocation)
& ([scriptblock]::Create((iwr https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1).Content)) -Version v0.3.0
```

What it does: downloads the Windows archive over HTTPS, verifies SHA256 against
`checksums.txt`, installs to `%LOCALAPPDATA%\Programs\mcp-server\mcp-server.exe`,
and registers a user-level Task Scheduler entry (or NSSM service if `nssm` is on
PATH). Reads the binary's fixed `.env` at `%USERPROFILE%\.config\mcp-appointments-crm\.env` when
present (same path on every OS; not `%APPDATA%`).

> **SmartScreen note (unsigned EXE)**: because the EXE is downloaded from the
> internet it carries MotW and SmartScreen will show **"Windows protected your
> PC" / "Unknown publisher"**. Click **More info → Run anyway**. This is expected
> for an unsigned binary and does not indicate a problem. The warning will only
> disappear if the project later ships a signed binary from a publisher with
> established SmartScreen reputation (OV ~$80–300/yr, EV ~$300–700/yr plus
> vetting). To avoid the dialog entirely, use the `go install` path above.

Verification:

```powershell
Invoke-RestMethod http://127.0.0.1:3000/mcp
Get-ScheduledTask -TaskName "mcp-appointments-crm" | Get-ScheduledTaskInfo
Get-Content "$env:LOCALAPPDATA\MCP Appointments CRM\Logs\mcp-server.log" -Tail 50
```

## Update & Rollback

### Update to a newer release

There is no `latest` resolution: pick the newest published tag and pass it explicitly.

```bash
# Linux / macOS — re-run installer pinned to the target tag (idempotent, keeps .env and data)
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0

# Windows primary — NOT IMPLEMENTED (target design only; see Install — Windows)
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@latest
```

### Update to a specific version

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0

# Windows primary — NOT IMPLEMENTED (target design only; see Install — Windows)
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0
```

### Rollback (binary)

Re-run the installer pinned to the **previous** tag (data is untouched — it lives
in `~/.local/share/mcp-appointments-crm/` outside the binary path):

```bash
# example: rollback from v0.4.0 to v0.3.0
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0
systemctl --user restart mcp-appointments-crm
curl --fail http://127.0.0.1:3000/healthz

# Windows — NOT IMPLEMENTED (target design only; see Install — Windows)
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0
Restart-ScheduledTask -TaskName "mcp-appointments-crm"
```

< 5 min per PRD §3.6.

### Rollback (data)

If the database must be restored, use the latest backup produced by
`scripts/backup.sh` (the operator schedules it — ADR-0003):

```bash
systemctl --user stop mcp-appointments-crm
gunzip -c ~/.local/share/mcp-appointments-crm/backups/reservas-20260827.db.gz \
  > ~/.local/share/mcp-appointments-crm/reservas.db
systemctl --user start mcp-appointments-crm
curl --fail http://127.0.0.1:3000/healthz
```

## Verification Checklist

Run after every install or update on Linux/macOS. All should pass. (Windows items are
not implemented — see the Install — Windows section note.)

```bash
# 1. Docker is NOT required (ADR-0001)
docker --version  # should be absent or irrelevant; service runs without it

# 2. Firewall — only loopback. No public port should be open for MCP.
sudo ufw status numbered 2>/dev/null || sudo iptables -L -n | head -20
# If UFW is active, ensure it doesn't block loopback (it doesn't by default).
# No rule should expose 3000 publicly. The binary itself rejects non-loopback binds.

# 3. Hardening (recommended for any VPS)
systemctl is-active fail2ban 2>/dev/null || echo "fail2ban not active — install if this is a public VPS"
systemctl is-active unattended-upgrades 2>/dev/null || echo "unattended-upgrades not active — enable for security patches"

# 4. Linger (Linux) — service survives logout
loginctl show-user $USER -p Linger  # expect Linger=yes

# 5. Service is active
systemctl --user is-active mcp-appointments-crm  # Linux: active
# macOS: launchctl list | grep com.mcp.appointments
# Windows: NOT IMPLEMENTED — target design only (Get-ScheduledTask -TaskName "mcp-appointments-crm")

# 6. Port and bind
ss -tlnp | grep 3000  # should show 127.0.0.1:3000, NOT 0.0.0.0:3000
curl --fail http://127.0.0.1:3000/healthz
# non-loopback must fail at startup (ADR-0007):
#   MCP_BIND=0.0.0.0 mcp-server  # → Error: MCP_BIND=0.0.0.0 expone el server…

# 7. Version and checksums
mcp-server --version
# compare archive sha256 against checksums.txt from the release:
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/checksums.txt
sha256sum -c checksums.txt --ignore-missing

# 8. Hermes integration (if Hermes is installed on the same host)
hermes doctor  # or equivalent — should report MCP endpoint reachable at 127.0.0.1:3000
```

## Security Notes

- **HTTPS only**: all release assets are fetched over HTTPS from `github.com` and
  `raw.githubusercontent.com`. No plain HTTP.
- **Checksums**: every install path verifies SHA256 against `checksums.txt` from
  the same GitHub Release before extraction/execution. Abort on mismatch.
- **Loopback enforcement**: the binary validates `MCP_BIND` at startup — only
  `127.0.0.0/8` and `::1` are accepted (ADR-0007). `0.0.0.0` or any LAN/public IP
  fails before the socket is opened. The service templates ship with
  `MCP_BIND=127.0.0.1` by default.
- **No root / no Docker**: the service runs as the invoking user (ADR-0002) with
  no `sudo` at any point. No container runtime (ADR-0001).
- **Version provenance**: `mcp-server --version` prints the ldflags-injected tag
  as a bare version string (e.g. `v0.3.0`). Compare it against the requested
  GitHub Release tag during install/upgrade verification.

## Troubleshooting

### Port already in use

```
Error: puerto 3000 en uso. Configurá MCP_PORT con otro valor (ej. export MCP_PORT=3001 && mcp-server).
```

The server does **not** auto-fallback to the next port (ADR-0007). Fix:

```bash
# option A: env var for this shell
MCP_PORT=3001 mcp-server
# option B: persist in .env (systemd reads it via EnvironmentFile)
echo "MCP_PORT=3001" >> ~/.config/mcp-appointments-crm/.env
systemctl --user restart mcp-appointments-crm
curl --fail http://127.0.0.1:3001/healthz
# option C: find who holds 3000
ss -tlnp | grep 3000
lsof -i :3000
```

### Bind is not loopback

```
Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).
Error: MCP_BIND=192.168.1.5 no es una dirección loopback. Use 127.0.0.1 (IPv4) o ::1 (IPv6).
Error: MCP_BIND=localhost es un hostname, no una IP. Use la IP literal (127.0.0.1 o ::1).
```

Fix: set `MCP_BIND=127.0.0.1` (or `::1`) in `~/.config/mcp-appointments-crm/.env`
and restart the service. Never use `0.0.0.0`.

### Windows SmartScreen "Unknown publisher"

**Not reproducible today**: the path this describes (`install.ps1` downloading an
unsigned prebuilt EXE) is not implemented and no Windows asset is published. The note
below is retained as target-design context. To avoid the dialog once Windows ships,
the `go install` path builds locally and never carries MotW. A signed binary would
still require OV/EV cost plus reputation warm-up and is deferred to a future decision.

### Service stops after logout (Linux)

Cause: `loginctl enable-linger` was not set. Fix:

```bash
loginctl enable-linger $USER
loginctl show-user $USER -p Linger  # Linger=yes
systemctl --user enable --now mcp-appointments-crm
```

`install.sh` runs this automatically; manual installs that bypass the script must
run it by hand.

### `install.sh` reports SHA256 mismatch

Do not bypass — the download is corrupt or tampered. Re-run the installer; if
it persists, download the archive and `checksums.txt` manually and compare:

```bash
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/mcp-appointments-crm_Linux_x86_64.tar.gz
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

If the official `checksums.txt` itself fails verification, do not install and
open an issue at `https://github.com/egkike/mcp-appointments-crm/issues`.

### Hermes cannot reach MCP

```bash
curl -v http://127.0.0.1:3000/mcp
ss -tlnp | grep mcp-server
journalctl --user -u mcp-appointments-crm -n 100 --no-pager
journalctl --user -u mcp-appointments-crm -f
# verify Hermes config points at http://127.0.0.1:3000/mcp (not 0.0.0.0, not a LAN IP)
```

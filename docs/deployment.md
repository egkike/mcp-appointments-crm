# Deployment & Release Guide

> **Source of truth for releases**: GitHub Releases + GoReleaser.
> Rationale and trade-offs are in [ADR-0014](./architecture/0014-release-and-deploy-workflow.md).

## Overview

`mcp-appointments-crm` ships as a single Go binary (pure Go via `modernc.org/sqlite`,
no CGo, no Docker) for 5 targets. Every tagged version produces 5 archives + a
SHA256 `checksums.txt` on GitHub Releases. Install scripts download the correct
archive over HTTPS, verify its checksum, install to user-level paths, register a
user-level service, and verify health on `http://127.0.0.1:3000`.

- Repo: `https://github.com/egkike/mcp-appointments-crm`
- Install scripts (raw):
  - Unix: `https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh`
  - Windows: `https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1`
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

5. **GitHub Action runs** `.github/workflows/release.yml`:
   - `goreleaser build --clean` cross-compiles 5 binaries (`CGO_ENABLED=0`)
   - injects `ldflags` version (`internal/version.Version/Commit/Date`)
   - creates archives + `checksums.txt`
   - publishes GitHub Release `v0.3.0` with all 6 assets.
6. **Verify the release**:

   ```bash
   gh release view v0.3.0 --repo egkike/mcp-appointments-crm
   curl -fsSL https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/checksums.txt | cat
   mcp-server --version   # after installing that tag
   curl --fail http://127.0.0.1:3000/mcp
   ```

No manual asset upload. If the Action fails, delete the remote tag, fix, and re-tag:

```bash
git push --delete origin v0.3.0
git tag -d v0.3.0
# fix, then re-tag and push
```

Dry-run locally before tagging:

```bash
goreleaser check
goreleaser build --snapshot --clean
```

## Versioning

- **SemVer** `vMAJOR.MINOR.PATCH` (e.g. `v0.3.0`, `v1.0.0`). Tags must match `v*`.
- **Conventional Commits** drive the changelog (`feat:`, `fix:`, `docs:`, etc.).
  Breaking changes use `feat!:` / `fix!:` or `BREAKING CHANGE:` footer and bump MAJOR.
- Pre-releases use `vX.Y.Z-rc.N` / `vX.Y.Z-beta.N` and are published as
  GitHub pre-releases (not `latest`).

## Artifacts

Each `vX.Y.Z` release publishes 6 files:

| File | Platform | Arch | Service manager |
|---|---|---|---|
| `mcp-appointments-crm_Linux_x86_64.tar.gz` | linux | amd64 | systemd (`--user`) |
| `mcp-appointments-crm_Linux_arm64.tar.gz` | linux | arm64 | systemd (`--user`) |
| `mcp-appointments-crm_Darwin_x86_64.tar.gz` | darwin | amd64 | launchd |
| `mcp-appointments-crm_Darwin_arm64.tar.gz` | darwin | arm64 | launchd |
| `mcp-appointments-crm_Windows_x86_64.zip` | windows | amd64 | NSSM or Task Scheduler |
| `checksums.txt` | — | — | SHA256 for the 5 archives |

All archives contain the binary `mcp-server` (or `mcp-server.exe` on Windows),
service templates, and `scripts/backup.sh` / `scripts/install.sh` helpers.
Version is embedded via `ldflags`; verify with:

```bash
mcp-server --version
# mcp-server v0.3.0 (commit abc1234, built 2026-08-27T…)
```

## Install — Linux

### One-line (recommended)

```bash
# latest
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash

# pinned version
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0
```

### HomeLab VM example (Tailscale `100.95.242.72`)

The canonical deployment for this project is the Linux HomeLab VM reachable over
Tailscale. No Go toolchain is required on the VM.

```bash
# from your workstation, over Tailscale
ssh kike@100.95.242.72

# on the VM — run the installer
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash

# check
systemctl --user is-active mcp-appointments-crm
systemctl --user status mcp-appointments-crm --no-pager
curl --fail http://127.0.0.1:3000/mcp
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
| Logs | `~/.local/state/mcp-appointments-crm/mcp-server.log` |
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

# health — loopback only
curl --fail http://127.0.0.1:3000/mcp
# with custom port
curl --fail http://127.0.0.1:${MCP_PORT:-3000}/mcp

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

Same `curl | bash` path as Linux (detects `Darwin` via `uname -s`):

```bash
# latest
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash

# pinned
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0
```

Service registration uses `launchd`:

| Component | Path (macOS) |
|---|---|
| Binary | `~/.local/bin/mcp-server` |
| Data | `~/Library/Application Support/MCP Appointments CRM/` |
| Config | `~/Library/Application Support/MCP Appointments CRM/setup/` + `.env` |
| Logs | `~/Library/Logs/MCP Appointments CRM/mcp-server.log` |
| Agent plist | `~/Library/LaunchAgents/com.mcp.appointments.server.plist` |

Verification:

```bash
launchctl list | grep com.mcp.appointments
launchctl print gui/$UID/com.mcp.appointments.server
curl --fail http://127.0.0.1:3000/mcp
log show --predicate 'process == "mcp-server"' --last 5m
```

No `loginctl` step on macOS — user LaunchAgents persist after logout by default.

## Install — Windows

Two paths. **Primary is `go install`** (no SmartScreen "Unknown publisher" dialog,
no cert cost). Fallback is `install.ps1` (prebuilt EXE, shows unsigned warning).

### Primary — `go install` (recommended)

Requires Go once (`winget install Go.Go` or `scoop install go`):

```powershell
# latest
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@latest

# pinned version
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0

# verify
mcp-server --version
# ensure %USERPROFILE%\go\bin is on PATH

# register as user-level service (Task Scheduler; NSSM if available)
mcp-server --register-service
# or: mcp-server install-service

# health
Invoke-RestMethod http://127.0.0.1:3000/mcp
Get-ScheduledTask -TaskName "mcp-appointments-crm" | Get-ScheduledTaskInfo
```

Why this avoids SmartScreen: the binary is **built locally** (`go` fetches module
sources over HTTPS and compiles). It never arrives as a downloaded EXE, so it
never receives a Mark of the Web (MotW) alternate data stream and never triggers
the SmartScreen publisher-reputation interstitial. No OV/EV certificate required.

### Fallback — `install.ps1` (prebuilt EXE)

```powershell
# latest — downloads mcp-server_windows_amd64.exe from GitHub Releases
irm https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1 | iex

# pinned — avoid piping when pinning (parameterized invocation)
& ([scriptblock]::Create((iwr https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1).Content)) -Version v0.3.0
```

What it does: downloads the Windows archive over HTTPS, verifies SHA256 against
`checksums.txt`, installs to `%LOCALAPPDATA%\Programs\mcp-server\mcp-server.exe`,
and registers a user-level Task Scheduler entry (or NSSM service if `nssm` is on
PATH). Reads `%APPDATA%\MCP Appointments CRM\.env` when present.

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

### Update to latest

```bash
# Linux / macOS — re-run installer (idempotent, keeps .env and data)
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash

# Windows primary
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@latest
```

### Update to a specific version

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0

# Windows primary
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0
```

### Rollback (binary)

Re-run the installer pinned to the **previous** tag (data is untouched — it lives
in `~/.local/share/mcp-appointments-crm/` outside the binary path):

```bash
# example: rollback from v0.4.0 to v0.3.0
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0
systemctl --user restart mcp-appointments-crm
curl --fail http://127.0.0.1:3000/mcp

# Windows
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0
Restart-ScheduledTask -TaskName "mcp-appointments-crm"
```

< 5 min per PRD §3.6.

### Rollback (data)

If the database must be restored, use the latest backup produced by
`scripts/backup.sh` (the operator schedules it — ADR-0005):

```bash
systemctl --user stop mcp-appointments-crm
gunzip -c ~/.local/share/mcp-appointments-crm/backups/reservas-20260827.db.gz \
  > ~/.local/share/mcp-appointments-crm/reservas.db
systemctl --user start mcp-appointments-crm
curl --fail http://127.0.0.1:3000/mcp
```

## Verification Checklist

Run after every install or update. All should pass.

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
# Windows: Get-ScheduledTask -TaskName "mcp-appointments-crm"

# 6. Port and bind
ss -tlnp | grep 3000  # should show 127.0.0.1:3000, NOT 0.0.0.0:3000
curl --fail http://127.0.0.1:3000/mcp
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
- **Version provenance**: `mcp-server --version` prints the ldflags-injected tag,
  commit SHA, and build date. Compare against `git rev-parse HEAD` and the GitHub
  Release tag.

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
curl --fail http://127.0.0.1:3001/mcp
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

Expected for the `install.ps1` (prebuilt unsigned EXE) path. See Windows section
above. To avoid it entirely, use `go install` which builds locally and never
carries MotW. If you must use the EXE, click **More info → Run anyway**. The
install script prints the same guidance. A signed binary would still require
OV/EV cost plus reputation warm-up and is deferred to a future decision.

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
cat ~/.local/state/mcp-appointments-crm/mcp-server.log
# verify Hermes config points at http://127.0.0.1:3000/mcp (not 0.0.0.0, not a LAN IP)
```

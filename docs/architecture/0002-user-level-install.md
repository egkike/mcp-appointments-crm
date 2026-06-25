# ADR-0002: User-level install with XDG / platform-native paths

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

After removing Docker (see [ADR-0001](./0001-no-docker.md)), the install model
was a system-level Unix-style setup: root required, dedicated `appuser`, files
in `/opt/mcp-server/`, service unit in `/etc/systemd/system/`. This was the
implicit assumption in the original PRD.

The project targets three deployment scenarios that the system-level model
fails to cover cleanly:
- **Linux VPS** (Ubuntu/Debian, the primary deployment)
- **macOS personal computers** (a barber shop owner running on their Mac)
- **Windows personal computers** (medical office running on Windows)

The personal-computer use case is blocked by `sudo` and `appuser` requirements.
The Linux-only `/opt/mcp-server/` convention does not translate to
`%APPDATA%` on Windows or `~/Library/Application Support/` on macOS. Each OS
needs a custom translation.

Modern self-hosted CLI tools (`rustup`, `mise`, `gh`, `pipx`, `bun`, `deno`)
all adopt the user-level install pattern. This is the convention the target
audience expects.

## Decision

The install is **user-level**: no `sudo`, no `appuser`, no system paths. The
service runs as the same user that invoked `install.sh`. Paths follow
platform conventions:

| Component | Linux (XDG) | macOS | Windows |
|---|---|---|---|
| Binary | `~/.local/bin/mcp-server` | `~/.local/bin/mcp-server` | `%LOCALAPPDATA%\Programs\mcp-server\mcp-server.exe` |
| Data (SQLite + backups) | `~/.local/share/mcp-appointments-crm/` | `~/Library/Application Support/MCP Appointments CRM/` | `%APPDATA%\MCP Appointments CRM\` |
| Config (JSON from wizard) | `~/.config/mcp-appointments-crm/setup/` | `~/Library/Application Support/MCP Appointments CRM/setup/` | `%APPDATA%\MCP Appointments CRM\setup\` |
| Logs | `~/.local/state/mcp-appointments-crm/mcp-server.log` | `~/Library/Logs/MCP Appointments CRM/mcp-server.log` | `%LOCALAPPDATA%\MCP Appointments CRM\Logs\mcp-server.log` |
| Service definition | `~/.config/systemd/user/mcp-appointments-crm.service` | `~/Library/LaunchAgents/com.mcp.appointments.server.plist` | Task Scheduler user task |

For 24/7 service on a Linux VPS, `install.sh` runs `loginctl enable-linger
<user>` automatically (one-time, no impact on login).

## Consequences

**Positive**:
- Zero `sudo` / root at any point during install
- No `appuser` concept to maintain (5 PRD references removed)
- Backups of the data dir become trivial (Time Machine, Windows Backup,
  `rsync` of `$HOME`)
- Each OS uses its own native convention — no awkward Unix-to-Windows path
  translation
- Matches the convention of modern CLI tools (low learning curve)
- The service can be inspected and managed with standard user tools
  (`systemctl --user`, `launchctl list`, Task Scheduler UI)

**Negative**:
- On Linux, a user-level systemd service stops on logout unless
  `loginctl enable-linger` is set (handled by `install.sh`)
- A single user can only have one install per machine (acceptable: the
  project is single-tenant by design)
- Cannot run as a system service visible to other users (acceptable for the
  target use case)

**Rejected alternatives**:
- **System-level install (`/opt`, `appuser`, `/etc/systemd/system/`)**: works
  on Linux VPS but blocks the personal-computer use case
- **Both options with a flag**: doubles the install script's complexity and
  test matrix; deferred to a future version if there's demand
- **Flatpak / Snap**: adds another runtime dependency, not aligned with
  "minimal dependencies"

## References

- `docs/PRD.md` §3.5 — install layout table
- `docs/PRD.md` §3.6 — rollback plan (uses user-level paths)
- `docs/PRD.md` §5.2 RNF Seguridad — service runs without root
- `docs/PRD.md` Fase 5 — install-and-service deliverables
- Commit `194888d` — "docs(prd): remove Docker, switch to user-level install"

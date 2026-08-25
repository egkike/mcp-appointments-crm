# MCP Appointments CRM

A high-performance, self-hosted, lightweight **MCP (Model Context Protocol) server**
for business bookings and CRM. Written in **Go**, backed by **SQLite** with FTS5.
Runs natively on Linux, macOS, and Windows — no containers, no external services.

## Status

🚧 **Pre-alpha** — under active development. Phases 1, 1b and 2 (mcp-server-core)
are complete; next up: Fase 3 (mcp-server-advanced). See the
[implementation roadmap](./docs/PRD.md#8-roadmap-por-fases) in the PRD.

The MCP server currently exposes 6 tools: `check_availability`, `create_booking`,
`get_booking`, `cancel_booking`, `reschedule_booking` and `get_business_profile`
(auth via `X-Caller-Id` header + RBAC, repository layer 10/10 repos with `auth.Caller` wiring — see PR #50).

| Phase | Description | Status |
|---|---|---|
| 1 | db-layer + clean-architecture-refactor (Fase 1b) | ✅ Done (archived 2026-08-09, commit `988baeb`) |
| 2 | mcp-server-core (Streamable HTTP + booking/profile tools) | ✅ Done (archived 2026-08-19, PRs #46/#47) |
| 2.5 | repository auth hardening (clients + BHE) | ✅ Done (PR #50, 2026-08-23 — 10/10 repos with `auth.Caller`) |
| 3 | mcp-server-advanced (alerts, loyalty, professional schedule) | ⏳ Next — ready (auth debt cleared) |
| 4 | config-wizard TUI (Bubble Tea) | ⏳ Planned |
| 5 | install-and-service (user-level, cross-platform) | ⏳ Planned |

## Quickstart

### Development

```bash
go run ./cmd/mcp-server   # exposes the MCP endpoint on http://127.0.0.1:3000/mcp (loopback only)
```

### Install — Linux / macOS

```bash
# latest
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash

# pinned version
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0
```

Installs the prebuilt binary from GitHub Releases, verifies SHA256, registers a
user-level service (`systemd --user` on Linux, `launchd` on macOS), enables linger
on Linux, and checks health at `http://127.0.0.1:3000/mcp`.

### Install — Windows

```powershell
# recommended — builds locally, no SmartScreen "Unknown publisher" dialog, no cert cost
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@latest
# then register as a user-level service
mcp-server --register-service

# alternative — prebuilt EXE (shows SmartScreen warning for unsigned binary)
irm https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1 | iex
```

> See [docs/deployment.md](./docs/deployment.md) for the full runbook (HomeLab VM
> example, manual download, verification checklist, rollback, troubleshooting) and
> [ADR-0014](./docs/architecture/0014-release-and-deploy-workflow.md) for the
> release rationale (GoReleaser, 5 platforms, checksums, `go install` vs prebuilt
> EXE trade-offs).

## Architecture

- **Language**: Go 1.26.7 with `modernc.org/sqlite` (pure Go, no CGo)
- **Database**: SQLite with WAL mode, FTS5 full-text search, `busy_timeout=5000`
- **TUI**: [Charm Bubble Tea](https://github.com/charmbracelet/bubbletea) ecosystem
- **Transport**: MCP over Streamable HTTP (spec 2025-11-25) on `127.0.0.1:3000` (loopback only) — go-sdk v1.2.0, implemented (`feat-mcp-transport`, archived 2026-08-19)
- **Install model**: user-level (no root, no `appuser`, no Docker). XDG paths on Linux
  (`~/.local/share/`, `~/.config/`), platform-native on macOS/Windows. See
  [docs/PRD.md §3.5](./docs/PRD.md#35-affected-areas) for the full install layout.

## Documentation

- [PRD](./docs/PRD.md) — Product Requirements Document (canonical source)
- [SDD](./docs/SDD.md) — original idea and analysis
- [PR Template](./docs/common/prd-template.md) — reusable template for new PRs
- [Architecture Decisions](./docs/architecture/) — ADRs
- [AGENTS.md](./AGENTS.md) — project conventions, coding standards, commit/PR process

## Development

Requires Go 1.26.7+.

```bash
go build -o /dev/null ./...        # compile
go test -v -race ./...             # tests
golangci-lint run ./...            # lint
```

Development is orchestrated with
[Gentle AI](https://github.com/Gentleman-Programming/gentle-ai) — the project
follows its spec-driven development (SDD) workflow and review gates
(`openspec/changes/`, judgment day, receipt-driven development on PRs).

## License

MIT License — see the [LICENSE](./LICENSE) file.

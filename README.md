# MCP Appointments CRM

A high-performance, self-hosted, lightweight **MCP (Model Context Protocol) server**
for business bookings and CRM. Written in **Go**, backed by **SQLite** with FTS5.
Runs natively on Linux, macOS, and Windows — no containers, no external services.

## Status

🚧 **Pre-alpha** — under active development. See the
[implementation roadmap](./docs/PRD.md#8-roadmap-por-fases) in the PRD.

| Phase | Description | Status |
|---|---|---|
| 1 | db-layer (extended schema, repository, FTS5 sync triggers) | ⏳ Planned |
| 2 | mcp-server-core (SSE + identity / resources / client / booking tools) | ⏳ Planned |
| 3 | mcp-server-advanced (alerts, loyalty, professional schedule) | ⏳ Planned |
| 4 | config-wizard TUI (Bubble Tea) | ⏳ Planned |
| 5 | install-and-service (user-level, cross-platform) | ⏳ Planned |

## Quickstart

> Will be available after Phase 5.

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash

# Windows (PowerShell)
iwr -useb https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1 | iex
```

## Architecture

- **Language**: Go 1.26.4 with `modernc.org/sqlite` (pure Go, no CGo)
- **Database**: SQLite with WAL mode, FTS5 full-text search, `busy_timeout=5000`
- **TUI**: [Charm Bubble Tea](https://github.com/charmbracelet/bubbletea) ecosystem
- **Transport**: MCP over SSE on `127.0.0.1:3000` (loopback only)
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

Requires Go 1.26.4+.

```bash
go build -o /dev/null ./...        # compile
go test -v -race ./...             # tests
golangci-lint run ./...            # lint
```

## License

TBD

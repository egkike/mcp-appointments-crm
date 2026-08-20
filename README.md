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
(auth via `X-Caller-Id` header + RBAC).

| Phase | Description | Status |
|---|---|---|
| 1 | db-layer + clean-architecture-refactor (Fase 1b) | ✅ Done (archived 2026-08-09, commit `988baeb`) |
| 2 | mcp-server-core (Streamable HTTP + booking/profile tools) | ✅ Done (archived 2026-08-19, PRs #46/#47) |
| 3 | mcp-server-advanced (alerts, loyalty, professional schedule) | ⏳ Planned |
| 4 | config-wizard TUI (Bubble Tea) | ⏳ Planned |
| 5 | install-and-service (user-level, cross-platform) | ⏳ Planned |

## Quickstart

Run the server in development:

```bash
go run ./cmd/mcp-server   # exposes the MCP endpoint on http://127.0.0.1:3000/mcp (loopback only)
```

> A `curl | bash` install script (`scripts/install.sh`, PowerShell `install.ps1` on
> Windows) will be shipped with the config wizard and service registration in
> Phases 4–5. No install commands are shown yet because they do not exist —
> running them would fail.

## Architecture

- **Language**: Go 1.26.4 with `modernc.org/sqlite` (pure Go, no CGo)
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

Requires Go 1.26.4+.

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

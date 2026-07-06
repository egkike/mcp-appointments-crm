# Architecture Decision Records (ADRs)

This directory contains the project's Architecture Decision Records (ADRs). Each ADR
documents a significant architectural decision: the **context** that forced it, the
**decision** taken, and the **consequences** (positive and negative).

## Format

We use a lightweight ADR format inspired by Michael Nygard's template:

- **Status**: `proposed` | `accepted` | `deprecated` | `superseded by ADR-NNNN`
- **Date**: YYYY-MM-DD
- **Context**: the forces at play (technical, business, philosophical)
- **Decision**: the response to those forces
- **Consequences**: trade-offs accepted and rejected alternatives
- **References**: links to related docs, PRs, commits

## Index

| ADR | Title | Status | Date |
|---|---|---|---|
| [0001](./0001-no-docker.md) | No Docker in deployment | accepted | 2026-06-25 |
| [0002](./0002-user-level-install.md) | User-level install with XDG / platform-native paths | accepted | 2026-06-25 |
| [0003](./0003-portable-backup.md) | Portable backup.sh, no auto-configured scheduler | accepted | 2026-06-25 |
| [0004](./0004-naming-conventions.md) | Project naming conventions | accepted | 2026-06-25 |
| [0005](./0005-optional-external-tools.md) | Project does not install external system tools; only suggests | accepted | 2026-06-25 |
| [0006](./0006-data-model-and-reservations.md) | Data model and reservation flow design | accepted | 2026-06-25 |
| [0007](./0007-server-config.md) | Server bind and port configuration | accepted | 2026-06-25 |
| [0008](./0008-install-prompts.md) | Inline prompts in install.sh (no separate config-wizard TUI) | accepted | 2026-06-26 |
| [0009](./0009-authorization-model.md) | Authorization model — `accounts` whitelist for admin/staff, `X-Caller-Id` header injection | accepted | 2026-06-29 |
| [0010](./0010-admin-tui.md) | TUI menú operacional (`mcp-appointments-crm admin tui`) for admin/staff account management | accepted | 2026-06-29 |
| [0011](./0011-owner-as-client.md) | Owner/admin/staff pueden ser clientes del negocio (mismo phone, doble rol) — `CallerResolver` combina accounts+clients info | accepted | 2026-06-29 |
| [0012](./0012-hermes-chat-local.md) | Segundo canal de comunicación — Chat nativo de Hermes (operación local, sub-comando del binario) con `MCP_CALLER_ID` env var override | accepted | 2026-06-29 |

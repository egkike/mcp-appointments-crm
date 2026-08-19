# Delta for architecture

> **Change**: feat-mcp-transport
> **Domain**: architecture (MODIFIED — new adapter layer)
> **Status**: Specified
> **Date**: 2026-08-05

## ADDED Requirements

### REQ-ARCH-INTMCP-001 — New adapter layer internal/mcp/

A new adapter layer `internal/mcp/` MUST be added to the Hexagonal model. Its role is the MCP transport: Streamable HTTP server, JSON-RPC 2.0 framing, tool registration, request/response mapping.

#### Scenario: Layer exists
- GIVEN the project structure
- WHEN reviewed
- THEN `internal/mcp/` MUST exist with at least one `.go` file implementing the MCP transport

### REQ-ARCH-INTMCP-002 — Composition root remains cmd/

`cmd/mcp-server/main.go` MUST remain the only composition root. It wires `internal/mcp/` adapters to `internal/application/usecase/` ports.

#### Scenario: Wiring in cmd/
- GIVEN the composition root
- WHEN reviewed
- THEN `cmd/mcp-server/main.go` MUST construct the MCP transport and inject use case interfaces

### REQ-ARCH-INTMCP-003 — Consumer interfaces declared in internal/mcp/

`internal/mcp/` MUST declare the consumer interfaces it needs (per data-access C5). The transport MUST NOT import `internal/repository/` directly.

#### Scenario: No direct repository import
- GIVEN the source of `internal/mcp/`
- WHEN imports are reviewed
- THEN `internal/repository` MUST NOT appear in any production `.go` file

### REQ-ARCH-INTMCP-004 — Adapter conventions

The new layer MUST follow existing adapter conventions: structured `log/slog`, Spanish error messages via `*domain.SemanticError`, `context.Context` propagation, `defer` for cleanup, `errors.Is` for sentinel checks.

#### Scenario: Structured logging used
- GIVEN the source of `internal/mcp/`
- WHEN error handling is reviewed
- THEN `*slog.Logger` MUST be used for all logging
- AND errors MUST be wrapped with `fmt.Errorf("...: %w", err)`

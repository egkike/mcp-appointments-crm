# Delta for auth-middleware

> **Change**: feat-mcp-transport
> **Domain**: auth-middleware (MODIFIED — wiring contract)
> **Status**: Specified
> **Date**: 2026-08-05

## ADDED Requirements

### REQ-AM-WIRED-001 — Middleware wrapped at composition root

At the composition root (`cmd/mcp-server/main.go`) the handler registered on `/mcp` MUST be the composed chain `jsonrpcAuthTranslator(auth.AuthMiddleware.Wrap(mcpHandler))`, per the design §4 (the JSON-RPC auth translator is the OUTERMOST handler, so the middleware is wrapped around the transport AND every request is processed by the middleware before reaching it).

#### Scenario: Auth chain wrapped around MCP handler
- GIVEN the composition root wiring
- WHEN reviewed
- THEN the handler registered on `/mcp` MUST be `jsonrpcAuthTranslator(authMiddleware.Wrap(mcpHandler))`, with the translator outermost so it can preserve the request `id` when re-emitting 401/403 as JSON-RPC

### REQ-AM-WIRED-002 — 401 translated to JSON-RPC error

When the middleware returns HTTP 401 (no/invalid `X-Caller-Id`), the MCP transport MUST translate it to a JSON-RPC 2.0 error response with code `-32000` and the Spanish message from the middleware.

#### Scenario: Missing header → JSON-RPC -32000
- GIVEN a POST to `/mcp` without `X-Caller-Id`
- WHEN processed
- THEN the response body MUST be a JSON-RPC 2.0 error with `code: -32000` and `message: "no se proporcionó X-Caller-Id"`

### REQ-AM-WIRED-003 — 403 translated to JSON-RPC error

When the middleware returns HTTP 403 (caller not authorized for the requested tool), the MCP transport MUST translate it to a JSON-RPC 2.0 error response with code `-32001` and the Spanish message.

#### Scenario: Insufficient role → JSON-RPC -32001
- GIVEN a caller with `Role: "client"` calling `create_booking` (requires owner/admin/staff)
- WHEN processed
- THEN the response body MUST be a JSON-RPC 2.0 error with `code: -32001` and `message: "no tienes permiso para realizar esta acción"`

### REQ-AM-WIRED-004 — Integration test for wired auth

An integration test MUST assert the wired behavior end-to-end: hitting `/mcp` without `X-Caller-Id` and asserting the 401-translated JSON-RPC error.

#### Scenario: E2E test asserts 401 → JSON-RPC
- GIVEN the test server is running with auth wired
- WHEN a POST to `/mcp` is made without `X-Caller-Id`
- THEN the response MUST be a JSON-RPC 2.0 error with `code: -32000`

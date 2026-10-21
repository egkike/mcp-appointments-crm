package mcp

// expectedToolCount is the single source of truth for the expected MCP tool
// registry size asserted across the mcp test suite: 8 core + 2 alerts + 1
// loyalty + 8 maintenance writes (ADR-0015) + 3 business-hours exception tools
// (feat-hours-exception-tool) + register_client (feat-whatsapp-bot D2, the
// anonymous self-registration tool) + 2 professionals read tools
// (feat-owner-chat S7a catalog read + S7b owner-only agenda read). The registry
// that produces this number is
// itself pinned by TestServerToolCountTracksRegisteredTools (server_test.go)
// through Server.ToolCount(); each transport-level fixture (e2e, integration)
// still proves that ITS OWN mux registers the full surface, but the expected
// value lives here so adding or removing a tool updates one literal instead of
// three.
const expectedToolCount = 25

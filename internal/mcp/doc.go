// Package mcp implements the MCP transport adapter mandated by ADR-0013: a
// Streamable HTTP (MCP 2025-11-25) server for the local Hermes client, bound
// to a loopback address only (REQ-MT-001). It owns the JSON-RPC 2.0 framing,
// tool registration and request/response mapping; the domain and use case
// layers never import this package.
package mcp

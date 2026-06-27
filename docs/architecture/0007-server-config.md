# ADR-0007: Server bind and port configuration

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

The MCP server binds to a TCP port on a loopback address to expose the
SSE endpoint to the local LLM client (Hermes). The original PRD
hardcoded `127.0.0.1:3000` with no configuration mechanism. After
review on 2026-06-25, the user identified two gaps:

1. **No port configurability**: if port 3000 is already in use
   (very common — Rails default, Grafana, Node dev servers, etc.),
   the operator has no way to use a different port without
   recompiling.
2. **No `.env` support**: the operator has to remember environment
   variable names and re-export them on every shell. There is no
   self-documenting config file.

This ADR documents the WHY of the configuration model implemented in
the PRD (§3.1, §3.5, §5.2, §6.3).

## Decision 1: Configuration sources with precedence (env vars > .env > defaults)

The server reads configuration from three sources, in this priority
order:

1. **System environment variables** (`MCP_BIND`, `MCP_PORT`)
2. **`~/.config/mcp-appointments-crm/.env`** (or platform-native
   equivalent per `docs/PRD.md §3.5 Install Layout`)
3. **Hardcoded defaults** (`127.0.0.1:3000`)

If the `.env` file doesn't exist, the server starts with the defaults
without error. This matches the 12-factor app convention (env vars are
first-class) while also providing a friendly local-dev story (the
operator edits a file instead of remembering shell commands).

**Rejected alternatives**:

- **Only env vars**: requires `export MCP_PORT=...` on every shell.
  The systemd service unit would need to inline the values via
  `Environment=`, making the service template verbose and
  non-discoverable.
- **Only `.env` file**: doesn't allow per-shell overrides (e.g.,
  "for this test, use port 4000"). Also breaks 12-factor compliance
  for containerized deployments.
- **TOML / YAML / JSON config**: heavier format. The two config
  values we need (`bind` and `port`) don't justify a structured
  format. `.env` is the simplest.

## Decision 2: No automatic port fallback

If the configured port is in use, the server **fails with a clear
error message** rather than trying the next free port.

**Rejected alternative**: try port 3000, if in use try 3001, 3002,
etc.

**Why the alternative is rejected**:

- **Predictability**: the operator configured a specific port
  (via `MCP_PORT` or `.env`). They expect the server to use that
  port. Falling back to "the next free one" violates the
  configuration.
- **Multi-instance support breaks**: two instances configured
  with `MCP_PORT=3000` would both fall back to the same next free
  port, colliding again.
- **Client configuration mismatch**: the operator's client
  (Hermes, curl, MCP config) is also configured with port 3000.
  A silent fallback to 3001 means the client can't find the
  server.
- **Discoverability**: how does the operator know the actual port?
  They have to read the server logs (if they have access) or
  grep the filesystem (if we write it somewhere).
- **Race conditions**: between the bind attempt on 3000 and the
  bind on 3001, another process could grab 3001.

**The error message** (per `docs/PRD.md §6.3`): the user-facing
error is a clear, actionable string like `Error: puerto 3000 en
uso. Configurá MCP_PORT con otro valor (ej. export
MCP_PORT=3001 && mcp-server).`

## Decision 3: `127.0.0.1` (literal IPv4), not `localhost` (hostname)

The default bind address is `127.0.0.1` (literal IPv4 loopback),
not `localhost`.

**Why `localhost` is rejected**:

The string `localhost` is a hostname, not an IP address. Its
resolution depends on the system's `/etc/hosts` and DNS resolver:

| System | `localhost` resolves to |
|---|---|
| macOS (default) | `::1` (IPv6) first, then `127.0.0.1` |
| Linux (with systemd-resolved) | `::1` first on some configs |
| Linux (with `nss-myhostname`) | `127.0.0.1` |
| Windows | depends on network config |

If the server binds to `localhost` and the OS resolves it to `::1`
(IPv6), but the client (Hermes, curl, MCP client) tries to connect
to `127.0.0.1` (IPv4), the connection is **refused**. The two are
on "loopback" but on different address families. This is a real
and common bug.

Binding to `127.0.0.1` (literal) means:
- The server listens on IPv4 loopback only
- Clients connecting to `127.0.0.1` always work
- Clients connecting to `::1` are correctly told "no" (they should
  use IPv4 anyway)
- Zero DNS resolution overhead

**`0.0.0.0` is FORBIDDEN**: that would bind to all interfaces,
including any public/LAN network. Inacceptable for a security-
sensitive service that should only be loopback.

## Decision 4: Bind validation against loopback at startup

`MCP_BIND` is validated to be a loopback address before the bind
attempt. Any non-loopback value fails the server with a security
error, before any network socket is opened.

**Valid loopback addresses**:

- IPv4: `127.0.0.0/8` (i.e., `127.0.0.1`, `127.0.0.2`, ..., `127.255.255.255`)
- IPv6: `::1` only

**Invalid values** (and the error they produce):

- `0.0.0.0` → `Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).`
- `192.168.1.5` → `Error: MCP_BIND=192.168.1.5 no es una dirección loopback. Use 127.0.0.1 (IPv4) o ::1 (IPv6).`
- `8.8.8.8` → same family of error
- `localhost` (if user passes it as a literal) → `Error: MCP_BIND=localhost es un hostname, no una IP. Use la IP literal (127.0.0.1 o ::1).`

The validation runs **before** the bind attempt, so a misconfigured
service fails fast without leaking any error from the OS network
stack.

## Decision 5: In-house `.env` parser, no external library

The `.env` parser will be implemented in `internal/config/dotenv.go`
(Fase 2+) as ~20 lines of Go (per [ADR-0005](./0005-optional-external-tools.md)
philosophy of "no external runtime tools"). The popular
`github.com/joho/godotenv` library is rejected. The spec `data-access`
defines the error types; the actual parser implementation is Fase 2 work.

**Why**:

- The format is trivial: `KEY=VALUE`, comments with `#`, optional
  quotes
- 20 lines of Go is faster to maintain than a third-party
  dependency
- No version-pinning, no security audit burden, no transitive
  deps
- We have ONE config file format; the library would handle 50
  edge cases we don't need

The parser handles:

- Lines with `KEY=VALUE` (whitespace trimmed)
- Lines starting with `#` (comments) → ignored
- Empty lines → ignored
- Values with surrounding `"` or `'` → quotes stripped

It does NOT handle: variable expansion, includes, multi-line
values, or command substitution. None of those are needed for our
two config values.

## Consequences

**Positive**:

- The server is configurable without recompiling
- The `.env` file is self-documenting (lists all available options)
- The systemd service unit can use `EnvironmentFile=` to load
  the same config
- Misconfigured binds fail safely with a clear, actionable error
- No external runtime dependencies (consistent with
  [ADR-0005](./0005-optional-external-tools.md))

**Negative**:

- Two more env vars to document (`MCP_BIND`, `MCP_PORT`) and
  remember
- The `.env` file location is OS-specific (XDG vs Apple vs
  Windows) — operator has to look it up
- The validation is duplicated logic (Go code + ADR); if the
  rules change, both have to be updated

**Rejected alternatives (global)**:

- **Unix domain socket instead of TCP**: removes the port
  concern entirely but breaks compatibility with any HTTP
  client (Hermes, MCP clients). Not worth it for MVP.
- **Service discovery (mDNS, Avahi)**: complex, adds a
  dependency. Not needed for a single-tenant install.
- **Config file in the data directory** (instead of config
  directory): mixes state with config. ADR-0002 established
  the separation.

## References

- `docs/PRD.md §3.1` In Scope — main endpoint statement
- `docs/PRD.md §3.5` Affected Areas — `.env` file path bullet
- `docs/PRD.md §5.2` RNF — Configurabilidad row
- `docs/PRD.md §6.3` Compliance y Seguridad — bind validation
- [ADR-0002](./0002-user-level-install.md) — XDG / platform-native paths
- [ADR-0005](./0005-optional-external-tools.md) — no external deps philosophy
- Commit `ba203e7` — when the PRD changes were applied

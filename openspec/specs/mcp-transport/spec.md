# MCP Transport Specification

> **Change**: feat-mcp-transport
> **Domain**: mcp-transport (NEW)
> **Status**: Specified
> **Date**: 2026-08-05

## Purpose

Serve the MCP protocol over Streamable HTTP (spec 2025-11-25) on `127.0.0.1:3000/mcp` for the local Hermes client. This is the transport adapter layer mandated by ADR-0013 — the Hexagonal "adapter" that wires existing use cases to the MCP protocol.

## Requirements

### REQ-MT-001 — Loopback bind only

The server MUST validate that `MCP_BIND` resolves to `127.0.0.0/8` or `::1` at startup. Non-loopback values MUST cause fail-fast with a Spanish error before `ListenAndServe`.

#### Scenario: Default bind succeeds
- GIVEN `MCP_BIND` is unset (default `127.0.0.1`)
- WHEN the server starts
- THEN it binds to `127.0.0.1:3000` successfully

#### Scenario: Non-loopback rejected
- GIVEN `MCP_BIND=0.0.0.0`
- WHEN the server starts
- THEN it MUST exit with `Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).`

### REQ-MT-002 — Single POST endpoint

The server MUST expose `POST /mcp` as the sole MCP endpoint. `GET /mcp` MUST return `405 Method Not Allowed`.

#### Scenario: POST /mcp accepted
- GIVEN a valid JSON-RPC 2.0 POST request to `/mcp`
- WHEN the server processes it
- THEN it returns a JSON-RPC 2.0 response

#### Scenario: GET /mcp rejected
- GIVEN a GET request to `/mcp`
- WHEN the server processes it
- THEN it returns HTTP 405

### REQ-MT-003 — JSON-RPC 2.0 framing

Every request/response MUST be a JSON-RPC 2.0 envelope. Malformed JSON MUST return error code `-32700` (Parse error).

#### Scenario: Malformed JSON
- GIVEN a POST body that is not valid JSON
- WHEN processed
- THEN response MUST contain `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`

### REQ-MT-004 — Initialize handshake

The server MUST respond to `initialize` with `protocolVersion: "2025-11-25"`, `serverInfo`, and `capabilities: {tools: {}}`.

#### Scenario: Initialize returns capabilities
- GIVEN a fresh client connection
- WHEN `initialize` is called
- THEN response includes `protocolVersion`, `serverInfo.name`, `serverInfo.version`, and `capabilities.tools`

### REQ-MT-005 — tools/list returns 25 tools

`tools/list` MUST return exactly 25 tool descriptors (see REQ-MT-015 for names and schemas).

(Previously: 24 tools — `feat-owner-chat` S7b adds the owner-only agenda read `get_professional_schedules`.)

(Previously: 23 tools — `feat-owner-chat` S7a adds the read tool `search_professionals_advanced`, readable by ANY authenticated caller and PII-free.)

(Previously: 20 tools — `feat-whatsapp-bot` adds `register_client` (19 → 20); that delta was backported into the base spec as a pre-step of this change; owner decision 2026-10-11.)

(Previously: 19 tools — ADR-0015 adds the 8 owner-only maintenance tools `update_business_profile`, `create_service`, `update_service`, `delete_service`, `create_professional`, `update_professional`, `upsert_schedule`, `delete_schedule`.)

(Previously: 11 tools — `feat-mcp-server-advanced` adds `search_clients_advanced`, `search_services_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`.)

(Previously: 6 tools — `feat-mcp-transport` baseline.)

This change adds `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions` (20 → 23).

`feat-owner-chat` S7a adds `search_professionals_advanced` (23 → 24); S7b adds `get_professional_schedules` (24 → 25).

#### Scenario: List returns all tools

- GIVEN a connected client
- WHEN `tools/list` is called
- THEN the response MUST contain 25 tools: `check_availability`, `create_booking`, `get_booking`, `cancel_booking`, `reschedule_booking`, `get_business_profile`, `search_clients_advanced`, `search_services_advanced`, `search_professionals_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`, `update_business_profile`, `create_service`, `update_service`, `delete_service`, `create_professional`, `update_professional`, `upsert_schedule`, `delete_schedule`, `register_client`, `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`, `get_professional_schedules`
### REQ-MT-006 — tools/call dispatch

`tools/call` MUST dispatch to the registered handler and return a JSON-RPC 2.0 result or error.

#### Scenario: Valid tool call
- GIVEN a `tools/call` for `check_availability` with valid args
- WHEN dispatched
- THEN the response MUST be a JSON-RPC result whose `result.content` is an array with exactly one item of shape `{"type": "text", "text": "<JSON of the typed tool output>"}` (SDK text-content envelope; the `text` payload is the JSON-serialized output object, e.g. `{"available": true}`)

#### Scenario: Unknown tool
- GIVEN a `tools/call` for `nonexistent_tool`
- WHEN dispatched
- THEN response MUST contain error code `-32601` (Method not found)

### REQ-MT-007 — Auth integration

Every request MUST pass through `auth.AuthMiddleware`. The resolved `Caller` MUST be propagated via `context.Context`.

#### Scenario: Caller propagated to handler
- GIVEN a request with valid `X-Caller-Id`
- WHEN the handler executes
- THEN `auth.FromContext(ctx)` returns the resolved `Caller`

#### Scenario: Invalid or unknown caller rejected before handler
- GIVEN a request with an `X-Caller-Id` that does not resolve to a known account (unknown ID, malformed format)
- WHEN processed
- THEN the response MUST be a JSON-RPC 2.0 error with code `-32000` and the resolver's Spanish message (auth layer rejects before any tool handler runs; `auth.FromContext(ctx)` is never reached)

### REQ-MT-008 — Auth errors as JSON-RPC

401/403 from the middleware MUST surface as JSON-RPC 2.0 errors with Spanish messages.

#### Scenario: 401 translated
- GIVEN a request without `X-Caller-Id`
- WHEN processed
- THEN response MUST contain JSON-RPC error with code `-32000` and message `"no se proporcionó X-Caller-Id"`

#### Scenario: 403 translated
- GIVEN a caller with insufficient role for the tool
- WHEN processed
- THEN response MUST contain JSON-RPC error with code `-32001` and message `"no tienes permiso para realizar esta acción"`

### REQ-MT-009 — Business errors in Spanish

Business-logic errors MUST surface as JSON-RPC 2.0 errors with the `*domain.SemanticError` Spanish message. No stack traces or raw SQL MUST leak.

#### Scenario: Overlap error
- GIVEN a `create_booking` that overlaps an existing booking
- WHEN dispatched
- THEN the JSON-RPC error message MUST be the domain's overlap message passed through verbatim: `"Profesional {id} ya tiene una reserva en ese horario"` with `{id}` = the professional ID as provided (`internal/application/usecase/create_booking.go`)

> **Template semantics**: `{id}` is an interpolation slot, not a literal character; the message carries no time window and uses the professional ID (not the name). `reschedule_booking` emits the sibling template `"Profesional {id} ya tiene una reserva en el nuevo horario"` on the same code path. The availability validator (check_availability result path) emits `"Profesional {name} ya tiene una reserva de {start} a {end}."` with `{name}` as stored and `{start}`/`{end}` RFC3339 UTC timestamps (`internal/domain/service/booking_time_validator.go`).
>
> **Amendment (2026-08-19, W-1)**: the original template `"el Profesional {name} ya tiene una reserva de {a} a {b}."` (HH:MM) described aspirational domain messages that do not exist in production; the domain messages above pre-date this change (present at base `0d9628e`) and this change's contract is verbatim passthrough of `*domain.SemanticError.Message` (REQ-MT-009 body). Precedent: REQ-MT-015 amendment (`faf431a`).

### REQ-MT-010 — Graceful shutdown

On SIGTERM/SIGINT, the server MUST call `http.Server.Shutdown(ctx)` with a 10-second deadline. The deadline MUST exceed the SQLite `_busy_timeout` (5000ms) so an in-flight non-idempotent mutation blocked on the write lock has margin to acquire it, commit, and return its JSON-RPC response before force-close. In-flight requests drain or are force-closed at the boundary.

#### Scenario: SIGTERM drains
- GIVEN an in-flight `tools/call`
- WHEN SIGTERM is received
- THEN the request completes within 10s or is force-closed

### REQ-MT-011 — Structured logging

Every request MUST log method, path, status, duration, and caller role (no PII). Errors MUST log structured fields.

#### Scenario: Request logged
- GIVEN a completed request
- WHEN the log is inspected
- THEN it contains `{method, path, status, duration_ms, caller_role}` fields

### REQ-MT-012 — Consumer interfaces in internal/mcp/

The transport MUST declare consumer interfaces in `internal/mcp/`. It MUST NOT import `internal/repository/` directly.

#### Scenario: No repository import
- GIVEN the source of `internal/mcp/`
- WHEN imports are reviewed
- THEN `internal/repository` MUST NOT appear

### REQ-MT-013 — Configuration via env vars

Configuration MUST use `MCP_BIND` + `MCP_PORT` with precedence per ADR-0007: explicit flag (reserved tier, none exist today) > env vars > `.env` > defaults. Defaults: `127.0.0.1:3000`.

#### Scenario: Custom port
- GIVEN `MCP_PORT=4000`
- WHEN the server starts
- THEN it binds to `127.0.0.1:4000`

### REQ-MT-014 — Health check

`GET /healthz` MUST return `200 OK` with JSON `{"status":"ok","version":"<x.y.z>"}`.

> **Liveness-only by design (accepted)** (RDD R3-010/R4-002): `/healthz` is deliberately a liveness probe, NOT a readiness probe — it does NOT check SQLite connectivity, WAL state, or schema version. Rationale: loopback-only, single trusted client (Hermes); DB failures surface immediately as `-32603` on the next `tools/call`, which is the client-visible signal. A separate readiness/DB probe is deferred to Fase 3 (Q-A4).

#### Scenario: Health check passes
- GIVEN the server is running
- WHEN `GET /healthz` is called
- THEN response is `200` with `{"status":"ok","version":"..."}`

#### Scenario: DB unreachable still reports liveness
- GIVEN SQLite is unreachable (disk full, WAL corruption, permissions)
- WHEN `GET /healthz` is called
- THEN response is STILL `200` `{"status":"ok","version":"..."}` (accepted liveness-only semantics; the DB failure surfaces as `-32603` on `tools/call`)

### REQ-MT-015 — Tool registry

| Tool | Roles | Input | Output |
|------|-------|-------|--------|
| `check_availability` | any authenticated (no RBAC entry = "any authenticated" per ToolRBAC contract, design §3) | `{service_id, professional_id, start_datetime, end_datetime?}` | `{available: bool, message?: string}` |
| `create_booking` | owner, admin, staff | `{client_id, service_id, professional_id, start_datetime, notes?}` | `{booking_id, start_datetime, end_datetime}` |
| `get_booking` | owner, admin, staff, client (self) | `{booking_id}` | `BookingView` — `{id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes?, payment_method?, created_at, updated_at}` |
| `cancel_booking` | owner, admin, staff | `{booking_id, reason}` | `{status: "cancelled"}` |
| `reschedule_booking` | owner, admin, staff | `{booking_id, new_start_datetime}` | `{booking_id, start_datetime, end_datetime}` |
| `get_business_profile` | owner, admin, staff | `{}` | `BusinessProfile` serialised via SDK output-schema inference from `entity.BusinessProfile` fields: `{id, name, industry?, country?, address?, latitude?, longitude?, cover_photo_url?, public_phone?, messenger_platform?, messenger_id?, contact_email?, website_url?, general_description?, currency_code, currency_symbol, accepted_payment_methods?, timezone, slot_interval_minutes, business_hours, created_at, updated_at, maps_url?}` (field SET pinned here) |
| `search_clients_advanced` | any authenticated (scoping in the use case/repo per the `get_booking` precedent — admin/owner all, client own row, staff bookings-scoped; see clients REQ-CL-AUTH-004) | `{query_text}` | `[]ClientSearchEntry` — `{id, name, phone, preferences?}` ordered by FTS relevance (bm25 ASC) |
| `search_services_advanced` | any authenticated (use case enforces owner/admin; other roles get a semantic forbidden error) | `{query_text}` | `[]ServiceSearchEntry` — `{id, name, description?, duration_minutes, price, is_active}` ordered by FTS relevance |
| `search_professionals_advanced` | any authenticated (no RBAC entry; the use case enforces only `auth.RequireAuthenticated` — deliberately NOT the owner/admin gate of `search_services_advanced`, because the client booking flow consumes it) | `{query_text?, service_id?}` (both optional; neither ⇒ full active catalog) | `{results: [{id, name, role_specialty?, specialties: [service_id…]}]}` — PII-free: `email`/`phone` MUST NOT appear. `service_id` filters fail-closed through `entity.Professional.HasSpecialty` (empty/unreadable `specialties` ⇒ empty list, never the full catalog); `query_text` is a case-insensitive substring over `name`/`role_specialty` validated by the shared FTS query validator; no FTS5 table is added |
| `get_professional_schedules` | owner (feat-owner-chat S7b — double enforcement: `ToolRBAC` entry AND `auth.RequireRole(RoleOwner)` in the use case) | `{professional_id}` | `{professional: {id, name, status}, schedules: [{id, day_of_week, start_time, end_time}]}` — every stored weekly slot ordered by `day_of_week` (0=domingo..6=sábado). Agenda rows carry NO `professional_id`: the response is scoped by `professional.id`, so a per-row copy is redundant (issued in #153, breaking wire change owner-approved 2026-10-30; the `upsert_schedule` read-back keeps the full stored-row shape). Inactive professionals ARE returned with their `status` (the delete gate must name whom it confirms). Unknown id ⇒ semantic NOT_FOUND; existing professional without slots ⇒ `schedules: []`. MUST NOT expose client or booking data |
| `get_pending_alerts` | owner, admin | `{}` | `[]PendingAlertView` — `{alert_id, type, message, scheduled_datetime, related_booking_id?}`: due pending only, oldest first |
| `mark_alert_as_sent` | owner, admin | `{alert_id}` | `{alert_id, status: "sent"}` |
| `get_loyalty_report` | owner, admin | `{period?, top_n?}` (defaults `last_month`, 10; see loyalty-report spec) | `[]LoyaltyReportEntry` — `{client_id, name, phone, booking_count}` ordered by `booking_count` DESC |
| `update_business_profile` | owner (ADR-0015) | partial merge: any subset of the 19 updatable profile fields as pointers (name, industry, country, address, latitude?, longitude?, cover_photo_url?, public_phone, messenger_platform?, messenger_id?, contact_email?, website_url?, general_description?, currency_code?, currency_symbol?, accepted_payment_methods?, timezone?, slot_interval_minutes?, business_hours?) — empty update rejected; `location_uri?` (RFC 5870 `geo:lat,long` URI) MAY be supplied as the transport-level alternative to the numeric `latitude`/`longitude` pair and MUST NOT be combined with them | `BusinessProfile` (same shape as `get_business_profile`, read-back) |
| `create_service` | owner (ADR-0015) | `{name, description?, duration_minutes, price, is_active? (default true)}` | `ServiceSearchEntry` with generated id |
| `update_service` | owner (ADR-0015) | `{service_id}` + pointer fields (name?, description?, duration_minutes?, price?, is_active?) | `ServiceSearchEntry` (merged) |
| `delete_service` | owner (ADR-0015) | `{service_id}` | `{service_id, status: "deleted"}`; FK from bookings ⇒ `-32002` "...reservas asociadas" |
| `create_professional` | owner (ADR-0015) | `{name, phone, specialities?, status?}` (status default `active`; specialities = service ids) | Professional with generated uuid id |
| `update_professional` | owner (ADR-0015) | `{professional_id}` + pointer fields (name?, phone?, specialities?, status?, role?, email?) | Professional (merged) |
| `upsert_schedule` | owner (ADR-0015) | `{professional_id, day_of_week (0=domingo..6=sábado), start_time, end_time}` HH:MM | Schedule as stored (id stable across re-upsert) |
| `delete_schedule` | owner (ADR-0015) | `{professional_id, day_of_week}` | `{professional_id, day_of_week, status: "deleted"}` |
| `register_client` | anonymous — no RBAC role entry; reachable ONLY through the static path allowlist evaluated before caller resolution, which requires a non-empty `X-Caller-Id` (see `auth-middleware` and `client-registration`) | `{display_name?}` — no field may set the phone; unknown fields are rejected | `{client_id, display_name, created}` (`client_id` is the header phone; `created = false` for an existing row) |
| `create_hours_exception` | owner (ADR-0015) | `{exception_date (YYYY-MM-DD), is_closed (bool), open_time? (HH:MM), close_time? (HH:MM), reason?}` — `open_time`/`close_time` are required when `is_closed=false` and forbidden when `true`; `open_time < close_time` | `HoursExceptionOut` — `{id, exception_date, is_closed, open_time?, close_time?, reason?}` (the stored row, read back so `id` is known; timestamps hidden per PRD §513, mirroring `serviceOut`) |
| `delete_hours_exception` | owner (ADR-0015) | `{exception_id (int)}` | `{exception_id, status: "deleted"}` |
| `list_hours_exceptions` | owner (ADR-0015) | `{from (YYYY-MM-DD), to (YYYY-MM-DD)}` — both required, range inclusive | `{exceptions: [HoursExceptionOut]}` ordered by `exception_date` ASC; empty range ⇒ `{exceptions: []}`, never an error. The object carrier (never a bare array) keeps the tool's `outputSchema` root `{"type": "object"}` — the only root the Hermes MCP client accepts (pydantic `mcp-types`, protocol `2025-11-25`; a slice root renders as `["null","array"]` and rejects the whole `ListToolsResult`, defect `defect-outputschema-array-root-contract`, v0.9.1) |

> **get_booking role semantics** (RDD R3-001): clients MAY retrieve their own bookings — the RBAC entry admits all four roles and `auth.AuthorizeBookingAccess` inside the use case enforces cross-tenant isolation (client → own bookings only; staff → linked professional's calendar; admin/owner → any).

> **New-tools role semantics** (Fase 3): the search tools carry no RBAC entry (any authenticated at the transport; role enforcement lives in the use case — `search_clients_advanced` per clients REQ-CL-AUTH-004, `search_services_advanced` owner/admin). The alert and loyalty tools carry explicit owner/admin RBAC entries (PII: alert payloads and loyalty rows expose client phones).

> **Maintenance-tools role semantics** (ADR-0015): the 8 maintenance tools are owner-only at BOTH layers (ToolRBAC row + `RequireRole(owner)` in the use case); no admin grant in the MVP. Semantic errors are LLM-facing Spanish (`-32002`); FK conflicts surface as business errors (delete_service ⇒ conflict "reservas asociadas"; upsert_schedule for a missing professional ⇒ not found "el profesional indicado no existe").

> **register_client role semantics** (feat-whatsapp-bot D2): the tool is the only one reachable without a resolved caller. It carries NO RBAC role entry in either layer and derives NO role from configuration, headers or the body; the only identity signal is the non-empty `X-Caller-Id` header, and the use case accepts no role parameter. Its guards are the `accounts` collision rejection and the per-phone rate limit (`client-registration`), not role checks.

> **Location contract fields** (feat-whatsapp-bot D4): `get_business_profile` gains the derived output field `maps_url`, present if and only if both `latitude` and `longitude` are set (value `https://maps.google.com/?q=<lat>,<long>`, plain decimal notation; omitted otherwise). `update_business_profile` gains the optional input `location_uri`, the `geo:lat,long` alternative to the numeric pair; the transport adapter parses it into the numeric fields so the use case keeps receiving normalized numbers only. Both are additive: without them the previous wire shapes stay valid. Full behavior is pinned in `business-profile`.

> **Exception-tools role semantics** (ADR-0015, feat-hours-exception-tool D5): the 3 tools are owner-only at BOTH layers — the `ToolRBAC` row `{auth.RoleOwner}` in the composition root **and** `auth.RequireRole(ctx, auth.RoleOwner)` inside each use case (AGENTS.md double enforcement). The repository's own `Create`/`Delete` guard remains `RequireRole(admin, owner)` and is NOT modified, so the layered guards are, from outer to inner: ToolRBAC `{owner}` → use-case `RequireRole(owner)` → repository `RequireRole(admin, owner)` → SQL `UNIQUE(exception_date)`. Admin is never granted through MCP (ADR-0015 Decision 2 keeps the partial admin scope deferred).
>
> **Exception-tools semantics** (feat-hours-exception-tool D1–D10): exceptions are **business-wide**, keyed by `exception_date` with no professional scoping (PRD §3.7.3; the table has no `professional_id`). There is **no update tool**: the repository documents exceptions as immutable and mandates delete + create to change a date (`internal/repository/business_hours_exception.go:19-22`, D3). Validation (date format/calendar validity, `is_closed`↔times consistency, `HH:MM`, `open < close`, uniqueness) stays in the repository and is **not** duplicated at the transport (ADR-0015 drift mitigation; D8). Semantic errors are LLM-facing Spanish `-32002`. The three tools take effect on the read path with **no read-path change**, because `check_availability` and `create_booking`/`reschedule_booking` already consume `BusinessHoursExceptionRepo.Get`.
>
> **`list_hours_exceptions` read scope note:** `REQ-BHE-AUTH-002` (capability `business-hours-exception`) keeps `Get`/`List` open to all authenticated callers at the **repository** layer because they sit on the availability hot path; the **MCP tool** `list_hours_exceptions` is nevertheless owner-only (maintenance surface). Both statements hold: the repository requirement is unchanged, only the transport surface is narrowed.

#### Scenario: Tool input validated

- GIVEN a `create_booking` call missing `client_id`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input

#### Scenario: Other required fields validated (RDD R3-007)

- GIVEN a `create_booking` call missing `service_id` (or `professional_id` / `start_datetime`)
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `get_booking` call missing `booking_id`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `cancel_booking` call missing `reason`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `reschedule_booking` call missing `new_start_datetime`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `check_availability` call missing required `start_datetime`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input

#### Scenario: check_availability required flags validated

- GIVEN a `check_availability` call with only `start_datetime` present (service_id/professional_id/end_datetime absent)
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `check_availability` call with `service_id`, `professional_id` and `start_datetime` present (`end_datetime` omitted)
- WHEN dispatched
- THEN it MUST succeed and return `{available: bool, message?: string}`

#### Scenario: New tool input validated (Fase 3)

- GIVEN a `search_clients_advanced` or `search_services_advanced` call with `query_text` missing or empty
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `mark_alert_as_sent` call missing `alert_id`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input

#### Scenario: RBAC enforced on owner/admin-only tools (Fase 3)

- GIVEN staff or client callers
- WHEN `get_pending_alerts`, `mark_alert_as_sent` or `get_loyalty_report` is called
- THEN the response MUST be a JSON-RPC error with code `-32001` and message `"no tienes permiso para realizar esta acción"` (REQ-MT-008)

#### Scenario: RBAC enforced on the hours-exception tools

- GIVEN an admin, staff or client caller
- WHEN `create_hours_exception`, `delete_hours_exception` or `list_hours_exceptions` is called
- THEN the response MUST be a JSON-RPC error with code `-32001` and message `"no tienes permiso para realizar esta acción"`
- AND no exception row MUST be created, deleted or returned

#### Scenario: Search tools admit any authenticated caller at the transport (Fase 3)

- GIVEN any authenticated caller (owner, admin, staff or client)
- WHEN `search_clients_advanced` or `search_services_advanced` is called
- THEN the transport MUST NOT reject on role; the result or the semantic scoping error comes from the use case layer

#### Scenario: Hours-exception input validated

- GIVEN a `create_hours_exception` call missing `exception_date` or `is_closed`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN an `is_closed: false` call without `open_time` or `close_time`, or with `open_time >= close_time`
- WHEN dispatched
- THEN the response MUST be a JSON-RPC error with code `-32002` and a semantic Spanish message, and no row MUST be written
- GIVEN a `delete_hours_exception` call missing `exception_id`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `list_hours_exceptions` call missing `from` or `to`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
### REQ-MT-016 — Spanish semantic errors for all tools

All tools MUST return `*domain.SemanticError` Spanish messages for business-rule failures (overlap, not-working-day, etc.).

#### Scenario: Not-working-day error
- GIVEN a booking attempt on a day the professional doesn't work
- WHEN dispatched
- THEN error message MUST be the domain's not-working-day message passed through verbatim, following the template `"Profesional {name} no trabaja los {día}."` with: `{name}` = the professional's name as stored (no case change, no leading article), `{día}` = the day of week in lowercase Spanish, plural form matching the article `los` (e.g. `domingos`, `martes` — `martes` is invariant)

> **Template semantics**: `{día}` is an interpolation slot rendered in the plural day form (the article `los` is fixed and the message has no `el` article before `Profesional`); the golden test asserts the fully substituted string (e.g. `"Profesional Juan no trabaja los domingos."`), sourced from `internal/domain/service/booking_time_validator.go`.
>
> **Amendment (2026-08-19, W-1)**: the original template `"el Profesional {name} no trabaja los {día}."` (with leading `el`) described an aspirational domain message that does not exist in production; the message above pre-dates this change (present at base `0d9628e`) and this change's contract is verbatim passthrough (REQ-MT-016 body). Precedent: REQ-MT-015 amendment (`faf431a`).

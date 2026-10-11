# Delta for mcp-transport

> **Change**: feat-hours-exception-tool · D1/D5/D8/D9/D10 (owner-only business-hours exception management)
> **Domain**: mcp-transport — the tool registry grows **20 → 23** (three new owner-only maintenance tools)
> **Type**: MODIFIED requirements only. Exactly two requirements change (REQ-MT-005 and REQ-MT-015); every other requirement, scenario and note of `openspec/specs/mcp-transport/spec.md` is preserved verbatim.
> **Baseline note (OQ-5, resolved 2026-10-11)**: the base spec `openspec/specs/mcp-transport/spec.md` was reconciled to the shipped **20 tools** as a pre-step of this change by backporting the archived `openspec/changes/archive/2026-10-08-feat-whatsapp-bot/specs/mcp-transport/spec.md` delta (19 → 20: `register_client`, plus `maps_url`/`location_uri`). This delta is authored against **20** and states **20 → 23**.

## MODIFIED Requirements

### REQ-MT-005 — tools/list returns 23 tools

`tools/list` MUST return exactly 23 tool descriptors (see REQ-MT-015 for names and schemas).

(Previously: 20 tools — `feat-whatsapp-bot` adds `register_client` (19 → 20); that delta was backported into the base spec as a pre-step of this change; owner decision 2026-10-11.)

(Previously: 19 tools — ADR-0015 adds the 8 owner-only maintenance tools `update_business_profile`, `create_service`, `update_service`, `delete_service`, `create_professional`, `update_professional`, `upsert_schedule`, `delete_schedule`.)

(Previously: 11 tools — `feat-mcp-server-advanced` adds `search_clients_advanced`, `search_services_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`.)

(Previously: 6 tools — `feat-mcp-transport` baseline.)

This change adds `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions` (20 → 23).

#### Scenario: List returns all tools

- GIVEN a connected client
- WHEN `tools/list` is called
- THEN the response MUST contain 23 tools: `check_availability`, `create_booking`, `get_booking`, `cancel_booking`, `reschedule_booking`, `get_business_profile`, `search_clients_advanced`, `search_services_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`, `update_business_profile`, `create_service`, `update_service`, `delete_service`, `create_professional`, `update_professional`, `upsert_schedule`, `delete_schedule`, `register_client`, `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`

### REQ-MT-015 — Tool registry

The registry MUST contain the following 23 tools (the full current-truth table; the three new rows are marked **[new]**):

| Tool | Roles | Input | Output |
|------|-------|-------|--------|
| `check_availability` | any authenticated (no RBAC entry = "any authenticated" per ToolRBAC contract) | `{service_id, professional_id, start_datetime, end_datetime?}` | `{available: bool, message?: string}` |
| `create_booking` | owner, admin, staff | `{client_id, service_id, professional_id, start_datetime, notes?}` | `{booking_id, start_datetime, end_datetime}` |
| `get_booking` | owner, admin, staff, client (self) | `{booking_id}` | `BookingView` — `{id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes?, payment_method?, created_at, updated_at}` |
| `cancel_booking` | owner, admin, staff | `{booking_id, reason}` | `{status: "cancelled"}` |
| `reschedule_booking` | owner, admin, staff | `{booking_id, new_start_datetime}` | `{booking_id, start_datetime, end_datetime}` |
| `get_business_profile` | owner, admin, staff | `{}` | `BusinessProfile` — `{id, name, industry?, country?, address?, latitude?, longitude?, cover_photo_url?, public_phone?, messenger_platform?, messenger_id?, contact_email?, website_url?, general_description?, currency_code, currency_symbol, accepted_payment_methods?, timezone, slot_interval_minutes, business_hours, created_at, updated_at, maps_url?}` |
| `search_clients_advanced` | any authenticated (scoping in the use case/repo) | `{query_text}` | `[]ClientSearchEntry` ordered by FTS relevance |
| `search_services_advanced` | any authenticated (use case enforces owner/admin) | `{query_text}` | `[]ServiceSearchEntry` ordered by FTS relevance |
| `get_pending_alerts` | owner, admin | `{}` | `[]PendingAlertView` — due pending only, oldest first |
| `mark_alert_as_sent` | owner, admin | `{alert_id}` | `{alert_id, status: "sent"}` |
| `get_loyalty_report` | owner, admin | `{period?, top_n?}` | `[]LoyaltyReportEntry` ordered by `booking_count` DESC |
| `update_business_profile` | owner (ADR-0015) | partial merge of the 19 updatable fields as pointers; `location_uri?` (`geo:lat,long`) MUST NOT be combined with numeric `latitude`/`longitude` | `BusinessProfile` (read-back) |
| `create_service` | owner (ADR-0015) | `{name, description?, duration_minutes, price, is_active? (default true)}` | `ServiceSearchEntry` with generated id |
| `update_service` | owner (ADR-0015) | `{service_id}` + pointer fields | `ServiceSearchEntry` (merged) |
| `delete_service` | owner (ADR-0015) | `{service_id}` | `{service_id, status: "deleted"}`; FK from bookings ⇒ `-32002` |
| `create_professional` | owner (ADR-0015) | `{name, phone, specialities?, status?}` | Professional with generated uuid id |
| `update_professional` | owner (ADR-0015) | `{professional_id}` + pointer fields | Professional (merged) |
| `upsert_schedule` | owner (ADR-0015) | `{professional_id, day_of_week, start_time, end_time}` | Schedule as stored (id stable across re-upsert) |
| `delete_schedule` | owner (ADR-0015) | `{professional_id, day_of_week}` | `{professional_id, day_of_week, status: "deleted"}` |
| `register_client` | anonymous — no RBAC entry; reachable ONLY through the static path allowlist (see `auth-middleware` and `client-registration`) | `{display_name?}` — no field may set the phone | `{client_id, display_name, created}` |
| **[new]** `create_hours_exception` | owner (ADR-0015) | `{exception_date (YYYY-MM-DD), is_closed (bool), open_time? (HH:MM), close_time? (HH:MM), reason?}` — `open_time`/`close_time` are required when `is_closed=false` and forbidden when `true`; `open_time < close_time` | `HoursExceptionOut` — `{id, exception_date, is_closed, open_time?, close_time?, reason?}` (the stored row, read back so `id` is known; timestamps hidden per PRD §513, mirroring `serviceOut`) |
| **[new]** `delete_hours_exception` | owner (ADR-0015) | `{exception_id (int)}` | `{exception_id, status: "deleted"}` |
| **[new]** `list_hours_exceptions` | owner (ADR-0015) | `{from (YYYY-MM-DD), to (YYYY-MM-DD)}` — both required, range inclusive | `[]HoursExceptionOut` ordered by `exception_date` ASC; empty range ⇒ `[]`, never an error |

> **Exception-tools role semantics** (ADR-0015, feat-hours-exception-tool D5): the 3 tools are owner-only at BOTH layers — the `ToolRBAC` row `{auth.RoleOwner}` in the composition root **and** `auth.RequireRole(ctx, auth.RoleOwner)` inside each use case (AGENTS.md double enforcement). The repository's own `Create`/`Delete` guard remains `RequireRole(admin, owner)` and is NOT modified, so the layered guards are, from outer to inner: ToolRBAC `{owner}` → use-case `RequireRole(owner)` → repository `RequireRole(admin, owner)` → SQL `UNIQUE(exception_date)`. Admin is never granted through MCP (ADR-0015 Decision 2 keeps the partial admin scope deferred).
>
> **Exception-tools semantics** (feat-hours-exception-tool D1–D10): exceptions are **business-wide**, keyed by `exception_date` with no professional scoping (PRD §3.7.3; the table has no `professional_id`). There is **no update tool**: the repository documents exceptions as immutable and mandates delete + create to change a date (`internal/repository/business_hours_exception.go:19-22`, D3). Validation (date format/calendar validity, `is_closed`↔times consistency, `HH:MM`, `open < close`, uniqueness) stays in the repository and is **not** duplicated at the transport (ADR-0015 drift mitigation; D8). Semantic errors are LLM-facing Spanish `-32002`. The three tools take effect on the read path with **no read-path change**, because `check_availability` and `create_booking`/`reschedule_booking` already consume `BusinessHoursExceptionRepo.Get`.
>
> **`list_hours_exceptions` read scope note:** `REQ-BHE-AUTH-002` (capability `business-hours-exception`) keeps `Get`/`List` open to all authenticated callers at the **repository** layer because they sit on the availability hot path; the **MCP tool** `list_hours_exceptions` is nevertheless owner-only (maintenance surface). Both statements hold: the repository requirement is unchanged, only the transport surface is narrowed.
>
> **Location contract fields** (D4, feat-whatsapp-bot): `get_business_profile` gains `maps_url` iff both coordinates are set; `update_business_profile` gains the optional `location_uri` `geo:lat,long` alternative. Both additive.

#### Scenario: Tool input validated

- GIVEN a `create_booking` call missing `client_id`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input

#### Scenario: Other required fields validated

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

- GIVEN a `check_availability` call with only `start_datetime` present
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `check_availability` call with `service_id`, `professional_id` and `start_datetime` present
- WHEN dispatched
- THEN it MUST succeed and return `{available: bool, message?: string}`

#### Scenario: New tool input validated

- GIVEN a `search_clients_advanced` or `search_services_advanced` call with `query_text` missing or empty
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input
- GIVEN a `mark_alert_as_sent` call missing `alert_id`
- WHEN dispatched
- THEN response MUST contain a JSON-RPC error indicating invalid input

#### Scenario: RBAC enforced on owner/admin-only tools

- GIVEN staff or client callers
- WHEN `get_pending_alerts`, `mark_alert_as_sent` or `get_loyalty_report` is called
- THEN the response MUST be a JSON-RPC error with code `-32001` and message `"no tienes permiso para realizar esta acción"` (REQ-MT-008)

#### Scenario: RBAC enforced on the hours-exception tools

- GIVEN an admin, staff or client caller
- WHEN `create_hours_exception`, `delete_hours_exception` or `list_hours_exceptions` is called
- THEN the response MUST be a JSON-RPC error with code `-32001` and message `"no tienes permiso para realizar esta acción"`
- AND no exception row MUST be created, deleted or returned

#### Scenario: Search tools admit any authenticated caller at the transport

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

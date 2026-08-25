# Delta for mcp-transport

> **Change**: feat-mcp-server-advanced · Fase 3 (RF3/RF7/RF8)
> **Domain**: mcp-transport — tool registry grows 6 → 11

## MODIFIED Requirements

### REQ-MT-005 — tools/list returns 11 tools

`tools/list` MUST return exactly 11 tool descriptors (see REQ-MT-015 for names and schemas).

(Previously: 6 tools — this change adds `search_clients_advanced`, `search_services_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`.)

#### Scenario: List returns all tools

- GIVEN a connected client
- WHEN `tools/list` is called
- THEN the response MUST contain 11 tools: `check_availability`, `create_booking`, `get_booking`, `cancel_booking`, `reschedule_booking`, `get_business_profile`, `search_clients_advanced`, `search_services_advanced`, `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report`

### REQ-MT-015 — Tool registry

| Tool | Roles | Input | Output |
|------|-------|-------|--------|
| `check_availability` | any authenticated (no RBAC entry = "any authenticated" per ToolRBAC contract, design §3) | `{service_id, professional_id, start_datetime, end_datetime?}` | `{available: bool, message?: string}` |
| `create_booking` | owner, admin, staff | `{client_id, service_id, professional_id, start_datetime, notes?}` | `{booking_id, start_datetime, end_datetime}` |
| `get_booking` | owner, admin, staff, client (self) | `{booking_id}` | `BookingView` — `{id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes?, payment_method?, created_at, updated_at}` |
| `cancel_booking` | owner, admin, staff | `{booking_id, reason}` | `{status: "cancelled"}` |
| `reschedule_booking` | owner, admin, staff | `{booking_id, new_start_datetime}` | `{booking_id, start_datetime, end_datetime}` |
| `get_business_profile` | owner, admin, staff | `{}` | `BusinessProfile` serialised via SDK output-schema inference from `entity.BusinessProfile` fields: `{id, name, industry?, country?, address?, latitude?, longitude?, cover_photo_url?, public_phone?, messenger_platform?, messenger_id?, contact_email?, website_url?, general_description?, currency_code, currency_symbol, accepted_payment_methods?, timezone, slot_interval_minutes, business_hours, created_at, updated_at}` (field SET pinned here) |
| `search_clients_advanced` | any authenticated (scoping in the use case/repo per the `get_booking` precedent — admin/owner all, client own row, staff bookings-scoped; see clients REQ-CL-AUTH-004) | `{query_text}` | `[]ClientSearchEntry` — `{id, name, phone, preferences?}` ordered by FTS relevance (bm25 ASC) |
| `search_services_advanced` | any authenticated (use case enforces owner/admin; other roles get a semantic forbidden error) | `{query_text}` | `[]ServiceSearchEntry` — `{id, name, description?, duration_minutes, price, is_active}` ordered by FTS relevance |
| `get_pending_alerts` | owner, admin | `{}` | `[]PendingAlertView` — `{alert_id, type, message, scheduled_datetime, related_booking_id?}`: due pending only, oldest first |
| `mark_alert_as_sent` | owner, admin | `{alert_id}` | `{alert_id, status: "sent"}` |
| `get_loyalty_report` | owner, admin | `{period?, top_n?}` (defaults `last_month`, 10; see loyalty-report spec) | `[]LoyaltyReportEntry` — `{client_id, name, phone, booking_count}` ordered by `booking_count` DESC |

> **get_booking role semantics** (RDD R3-001): clients MAY retrieve their own bookings — the RBAC entry admits all four roles and `auth.AuthorizeBookingAccess` inside the use case enforces cross-tenant isolation (client → own bookings only; staff → linked professional's calendar; admin/owner → any).

> **New-tools role semantics** (Fase 3): the search tools carry no RBAC entry (any authenticated at the transport; role enforcement lives in the use case — `search_clients_advanced` per clients REQ-CL-AUTH-004, `search_services_advanced` owner/admin). The alert and loyalty tools carry explicit owner/admin RBAC entries (PII: alert payloads and loyalty rows expose client phones).

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

#### Scenario: Search tools admit any authenticated caller at the transport (Fase 3)

- GIVEN any authenticated caller (owner, admin, staff or client)
- WHEN `search_clients_advanced` or `search_services_advanced` is called
- THEN the transport MUST NOT reject on role; the result or the semantic scoping error comes from the use case layer

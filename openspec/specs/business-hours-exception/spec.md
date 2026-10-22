# Spec: business-hours-exception

> Reference: `docs/PRD.md` §3.7.3; `docs/architecture/0006-data-model-and-reservations.md` Decisión 2
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe permitir al dueño del negocio registrar fechas específicas en las que el horario regular semanal no aplica (feriados, eventos especiales, vacaciones, días con horario reducido). Esta tabla es consultada por `check_availability` antes del JSON `business_hours` para responder "¿el negocio está abierto este día?".

## Requirements

### Requirement: One row per exception date

The system MUST enforce that `exception_date` is unique across the table. A second insert for the same calendar date MUST fail at the database level.

#### Scenario: Inserting a duplicate date fails

- GIVEN the table already has a row for `2026-12-25`
- WHEN an insert with `exception_date = '2026-12-25'` is attempted
- THEN the database MUST reject the statement with a unique-constraint violation

#### Scenario: Inserting a different date succeeds

- GIVEN the table already has a row for `2026-12-25`
- WHEN an insert with `exception_date = '2026-12-26'` is attempted
- THEN the insert MUST succeed and a subsequent SELECT MUST return both rows

### Requirement: ISO date format with no timezone component

The `exception_date` column MUST store calendar dates in the format `YYYY-MM-DD` (ISO 8601 date, no time component, no timezone offset). The column type MUST be `TEXT`.

#### Scenario: Date is stored as YYYY-MM-DD

- GIVEN a fresh table
- WHEN an exception for Christmas 2026 is inserted
- THEN the stored value MUST be exactly the string `2026-12-25`

#### Scenario: Rejecting date with time component

- GIVEN a fresh table
- WHEN an insert with `exception_date = '2026-12-25T00:00:00'` (not the canonical `YYYY-MM-DD`) is attempted
- THEN the repository MUST reject the input with `Code == ErrCodeInvalidInput`
- AND the database MUST NOT receive the INSERT

### Requirement: `is_closed` flag drives the open/close semantics

The `is_closed` column MUST be a boolean (`0` or `1`). When `is_closed = 1` the business is closed for that date; when `is_closed = 0` the business is open with a custom schedule.

#### Scenario: Closed exception with no open/close times

- GIVEN an exception row with `is_closed = 1`
- WHEN `check_availability` reads the row
- THEN the system MUST treat the business as closed regardless of the `business_hours` JSON for that weekday

#### Scenario: Open exception with custom times

- GIVEN an exception row with `is_closed = 0`, `open_time = '10:00'`, `close_time = '14:00'`
- WHEN `check_availability` reads the row
- THEN the system MUST use `10:00` and `14:00` as the opening window for that date, ignoring the JSON `business_hours` for the corresponding weekday

### Requirement: `open_time` and `close_time` consistency with `is_closed`

When `is_closed = 1`, the `open_time` and `close_time` columns MUST both be `NULL`. When `is_closed = 0`, both columns MUST be present, in `HH:MM` 24-hour format, and `open_time` MUST be strictly earlier than `close_time`.

#### Scenario: Closed exception stores NULL times

- GIVEN a fresh table
- WHEN a row is inserted with `is_closed = 1`
- THEN the stored `open_time` and `close_time` MUST both be `NULL`

#### Scenario: Open exception requires both times

- GIVEN a fresh table
- WHEN a row is inserted with `is_closed = 0` and only `open_time` (no `close_time`)
- THEN the application-level validation MUST reject the input with a semantic error; the database SHOULD also reject if a `CHECK` constraint is declared

#### Scenario: open_time must be earlier than close_time

- GIVEN a fresh table
- WHEN a row is inserted with `is_closed = 0`, `open_time = '18:00'`, `close_time = '09:00'`
- THEN the application-level validation MUST reject the input with a semantic error indicating that the open time is not before the close time

### Requirement: `reason` is optional free text

The `reason` column MUST accept a human-readable explanation of why the exception exists (for example `Navidad`, `Vacaciones del dueño`, `Feriado puente`). It MAY be `NULL` if no reason is provided.

#### Scenario: Reason stored

- GIVEN a fresh table
- WHEN a Christmas exception is inserted with `reason = 'Navidad'`
- THEN a subsequent SELECT MUST return that exact value for that row

#### Scenario: Reason omitted is allowed

- GIVEN a fresh table
- WHEN an exception is inserted with `reason = NULL`
- THEN the insert MUST succeed and the row MUST be returned with `reason = NULL` on subsequent SELECTs

### Requirement: Application-level rejection of malformed date formats

The repository MUST validate that date inputs (such as `exception_date` in `business_hours_exception`) match the canonical `YYYY-MM-DD` format before passing them to the database. Non-canonical date strings MUST NOT be silently stored.

#### Scenario: Rejects malformed exception_date format

- GIVEN an input with `exception_date` not matching `YYYY-MM-DD` (e.g., `2026-12-25T00:00:00` or `25/12/2026`)
- WHEN the repository is called
- THEN the call MUST return an error with `Code == ErrCodeInvalidInput`
- AND the database MUST NOT receive the INSERT

### Requirement: `check_availability` precedence rule

`check_availability` MUST consult `business_hours_exception` before the JSON `business_hours` for the date in question. If a row exists, the JSON MUST NOT be consulted.

#### Scenario: Exception overrides JSON

- GIVEN the JSON `business_hours` says `monday` is open from `09:00` to `18:00`
- AND an exception row exists for `2026-07-13` (which is a Monday) with `is_closed = 1`
- WHEN `check_availability` is called for a slot on `2026-07-13`
- THEN the system MUST return a semantic error indicating the business is closed for that date, not that it is open from 09:00 to 18:00

### REQ-BHE-AUTH-001 — Create and Delete are admin/owner-only (DoS prevention)

`Create` and `Delete` MUST require role `admin` or `owner`. An `is_closed = 1` row makes `check_availability` treat the business as closed for that date (§3.7.13 Paso 3a), so any caller able to write exceptions can suppress bookings by planting closing exceptions (e.g. a far-future date). Any other caller gets staff/client/unauth rejection. Authorized-caller semantics (`ErrInvalidInput` validation, `ErrConflict` on duplicate date, `ErrNotFound` on missing id) are unchanged.

#### Scenario: Admin creates an exception

- GIVEN an admin caller
- WHEN `Create(ctx, exception)` is called with a valid exception for `2026-12-25`
- THEN the exception MUST be persisted

#### Scenario: Admin deletes an exception

- GIVEN an admin caller and an existing exception for `2026-12-25`
- WHEN `Delete(ctx, id)` is called with that id
- THEN the exception MUST be removed

#### Scenario: Closing-exception planting blocked

- GIVEN a staff caller, a client caller, and a caller-less context
- WHEN any of them calls `Create` with a far-future closed exception or `Delete` with any id
- THEN staff/client/unauth rejection applies to each call, so no booking-suppressing exception can be planted

### REQ-BHE-AUTH-002 — Get and List are open to all authenticated callers

`Get` and `List` MUST accept any authenticated caller (`owner`, `admin`, `staff`, `client`) and MUST apply no row filter. Exception rows are calendar metadata without PII, and both methods sit on the `check_availability` / `create_booking` hot path (§3.7.13 Paso 3a): every role MUST be able to read the exception for a slot being booked. A caller-less context MUST receive `domain.ErrUnauthenticated`. Read semantics (`ErrNotFound` for a date without exception; inclusive range ordered by date) are unchanged.

#### Scenario: Client reads an exception on the booking hot path

- GIVEN a client caller and an exception for `2026-12-25` with `is_closed = 1`
- WHEN `Get(ctx, 2026-12-25)` is called
- THEN the exception MUST be returned

#### Scenario: Staff lists exceptions in a range

- GIVEN a staff caller and two exceptions inside `[2026-12-24, 2026-12-26]`
- WHEN `List(ctx, 2026-12-24, 2026-12-26)` is called
- THEN both MUST be returned, ordered by `exception_date` ascending

#### Scenario: Unauthenticated reads rejected

- GIVEN a caller-less context
- WHEN `Get` or `List` is called
- THEN the call MUST fail with `domain.ErrUnauthenticated`, without querying the database

#### Scenario: Auth gate does not break availability

- GIVEN an exception row for a date
- WHEN callers of all four roles resolve availability for that date through the booking flow
- THEN the read MUST succeed for every role (the gate must not break the `check_availability` hot path)

### REQ-BHE-MCP-001 — Owner-only management tools exist

The server MUST expose exactly three MCP tools for business-hours exception management: `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions`. Each MUST be owner-only at **both** layers: a `ToolRBAC` row of `{owner}` at the composition root and `auth.RequireRole(ctx, auth.RoleOwner)` inside the backing use case. No admin, staff or client caller may reach the repository through these tools.

The tools MUST be backed by the existing `BusinessHoursExceptionRepo` methods (`Create`, `List`, `Delete`) with **no repository change** and **no update tool**: the repository documents exceptions as immutable and mandates delete + create to change a date.

#### Scenario: Owner creates and deletes an exception

- GIVEN an owner caller
- WHEN `create_hours_exception` is called with `{exception_date: "2026-12-25", is_closed: true, reason: "Navidad"}`
- THEN the row MUST be persisted and the response MUST return the stored row including its generated `id`
- AND `delete_hours_exception` with that `id` MUST remove the row and return `{exception_id, status: "deleted"}`

#### Scenario: Non-owner callers are rejected

- GIVEN an admin, staff or client caller
- WHEN `create_hours_exception`, `delete_hours_exception` or `list_hours_exceptions` is called
- THEN the response MUST be `-32001` `"no tienes permiso para realizar esta acción"` and no row MUST be written, deleted or returned

#### Scenario: No update tool exists

- GIVEN the registered tool surface
- WHEN `tools/list` is inspected
- THEN there MUST be no `update_hours_exception` tool, and changing an exception MUST require `delete_hours_exception` followed by `create_hours_exception`

### REQ-BHE-MCP-002 — Create reuses the repository validation and surfaces Spanish semantic errors

`create_hours_exception` MUST validate through the repository only (single validator, no transport duplication): `exception_date` in `YYYY-MM-DD` and calendar-valid; `is_closed = true` ⇒ `open_time`/`close_time` absent; `is_closed = false` ⇒ both present in `HH:MM` with `open_time < close_time`; `exception_date` unique. Failures MUST reach the client as `*domain.SemanticError` messages (`-32002`) in Spanish, with no repository prefix, SQL text or driver detail. A duplicate date MUST fail with `ErrCodeConflict` and MUST NOT modify the existing row.

#### Scenario: Open exception with a custom window is stored

- GIVEN an owner caller
- WHEN `create_hours_exception` is called with `{exception_date: "2026-07-13", is_closed: false, open_time: "10:00", close_time: "14:00"}`
- THEN the row MUST be persisted with that window and `check_availability` for `2026-07-13` MUST use `10:00`–`14:00`

#### Scenario: Inconsistent closed/open payload is rejected

- GIVEN an owner caller
- WHEN `create_hours_exception` is called with `is_closed: true` plus `open_time`/`close_time`, or with `is_closed: false` and only one time, or with `open_time >= close_time`
- THEN the response MUST be a Spanish `-32002` error and no row MUST be written

#### Scenario: Malformed date is rejected

- GIVEN an owner caller
- WHEN `create_hours_exception` is called with `exception_date: "2026-12-25T00:00:00"` or `"25/12/2026"`
- THEN the response MUST be a Spanish `-32002` error and no row MUST be written

#### Scenario: Duplicate date conflicts

- GIVEN an existing exception for `2026-12-25`
- WHEN `create_hours_exception` is called for `2026-12-25`
- THEN the response MUST be `-32002` with an `ErrCodeConflict` Spanish message and the existing row MUST be unchanged

### REQ-BHE-MCP-003 — `list_hours_exceptions` reads an inclusive range

`list_hours_exceptions` MUST require both `from` and `to` (`YYYY-MM-DD`), MUST treat the range as inclusive, and MUST return the matching rows ordered by `exception_date` ascending, nested under the `exceptions` key of the response object. The response object is the **carrier** that keeps the tool's `outputSchema` root `{"type": "object"}`: the Hermes MCP client (pydantic `mcp-types`, protocol `2025-11-25`) validates that root as `Literal["object"]`, while a bare-slice output renders as the root `{"type": ["null", "array"]}`, which rejects the whole `ListToolsResult` and leaves the server without tools (defect `defect-outputschema-array-root-contract`, v0.9.1). An empty range MUST return `{"exceptions": []}`, never an error. A malformed bound MUST fail with a Spanish `-32002` `ErrCodeInvalidInput` message.

> The repository's `REQ-BHE-AUTH-002` (Get/List open to any authenticated caller because they sit on the availability hot path) is **unchanged**; only the MCP tool surface is owner-only (REQ-BHE-MCP-001).

#### Scenario: Range returns ordered rows

- GIVEN exceptions for `2026-12-24`, `2026-12-25` and `2026-12-26`
- WHEN an owner calls `list_hours_exceptions {from: "2026-12-24", to: "2026-12-26"}`
- THEN all three rows MUST be returned under `exceptions`, ordered by `exception_date` ascending

#### Scenario: Empty range is not an error

- GIVEN no exception in the requested range
- WHEN an owner calls `list_hours_exceptions` for that range
- THEN the response MUST be `{"exceptions": []}` and MUST NOT be an error

#### Scenario: Malformed bound is rejected

- GIVEN an owner caller
- WHEN `list_hours_exceptions` is called with a bound that is not `YYYY-MM-DD`
- THEN the response MUST be a Spanish `-32002` error

### REQ-BHE-MCP-004 — Delete by id and unknown-id semantics

`delete_hours_exception` MUST identify the row by its integer `id` (mirroring the repository `Delete(ctx, id)`). An id with no matching row MUST fail with a Spanish `-32002` `ErrCodeNotFound` message. A successful delete MUST return `{exception_id, status: "deleted"}`.

#### Scenario: Unknown id is a semantic not-found

- GIVEN no exception with `id = 999`
- WHEN an owner calls `delete_hours_exception {exception_id: 999}`
- THEN the response MUST be a Spanish `-32002` not-found message and no row MUST be affected

### REQ-BHE-MCP-005 — One audit record per mutation

Each successful `create_hours_exception` and `delete_hours_exception` MUST emit exactly one structured audit record at the use-case layer (`logMaintenanceMutation`), with `entity = "business_hours_exception"`, the affected row id as `entity_id`, and action `create` / `delete`. `list_hours_exceptions` is a read and MUST NOT emit an audit record. No caller identifier (PII) and no `reason` text MUST be logged beyond the row id.

#### Scenario: Create and delete are audited, list is not

- GIVEN an owner caller
- WHEN a create followed by a delete succeeds, and a list is called
- THEN exactly two audit records MUST be emitted (`create` with the new id, `delete` with that id)
- AND the list MUST emit none

### REQ-BHE-MCP-006 — Only semantic errors reach the client

Every failure of the three tools MUST reach the client as a `*domain.SemanticError` message (`-32002`) in Spanish. Any non-semantic error MUST collapse to the generic `-32603` and MUST NOT leak repository, SQL or filesystem detail. The repository's `SemanticError.Cause` (which may echo the raw date) MUST never be serialized.

#### Scenario: Internal failure is opaque

- GIVEN an internal repository failure not mapped to a semantic error
- WHEN any of the three tools is called
- THEN the client MUST receive `-32603` with a generic message and the detail MUST appear only in the server log

### REQ-BHE-MCP-007 — Created exceptions govern the read paths with no read-path change

An exception created through `create_hours_exception` MUST immediately govern `check_availability`, `create_booking` and `reschedule_booking`, because those paths already consume `BusinessHoursExceptionRepo.Get`. The MCP management tools MUST NOT require any modification to the availability service, the slot-context resolver or the booking use cases.

#### Scenario: Wire-created exception changes availability

- GIVEN the weekly `business_hours` says the business is open on a date
- WHEN an owner creates a closing exception (`is_closed = true`) for that date through the tool
- THEN `check_availability` for a slot on that date MUST report the business closed for the exception date
- AND deleting the exception MUST restore the weekly-schedule behavior

## Notes

- The unique index on `exception_date` is also the secondary index that makes the 3a step of `check_availability` an O(log n) lookup.
- **`created_at` is hidden from the wire** (owner decision 2026-10-11, OQ-3): the `create_hours_exception` / `list_hours_exceptions` tool output omits it, consistent with `serviceOut` and PRD §513's "sin timestamps" convention. The column remains in SQLite and in the repository row; only the wire shape drops it.
- **No national-holiday seed.** PRD §3.7.3 rejects an automatic holiday library; the owner loads the dates that matter (this is now possible through `create_hours_exception`).
- This table deliberately does NOT auto-populate from a national holiday library. The owner is expected to add the dates that affect their business. ADR-0006 Decisión 2 documents the rejected alternatives.
- See `bookings` capability for how this table is consumed by the 5-step chain.

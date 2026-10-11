# Delta: business-hours-exception (feat-hours-exception-tool)

> Reference: `openspec/changes/feat-hours-exception-tool/proposal.md` (D1–D10), `design.md` §1–§6, `docs/PRD.md` §3.7.3 / §3.7.13 Paso 3a, ADR-0006 Decision 2, ADR-0015
> Change: feat-hours-exception-tool
> Capability: **`business-hours-exception`** — existing base spec at `openspec/specs/business-hours-exception/spec.md` (REQ-BHE-AUTH-001/002 and the table/precedence requirements live there and are unchanged)
> Type: **ADDED requirements only** — the repository table, validations, precedence rule and the `REQ-BHE-AUTH-001/002` guards are unchanged. No schema change, no repository change.
>
> **Naming note (OQ-2, resolved 2026-10-11):** the change directory is `specs/business-hours-exception/` (singular), matching the base capability `openspec/specs/business-hours-exception/` and the archived-delta convention. The plural name from the delegation memo was superseded and the directory renamed by the orchestrator.

## ADDED Requirements

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

`list_hours_exceptions` MUST require both `from` and `to` (`YYYY-MM-DD`), MUST treat the range as inclusive, and MUST return the matching rows ordered by `exception_date` ascending. An empty range MUST return an empty array, never an error. A malformed bound MUST fail with a Spanish `-32002` `ErrCodeInvalidInput` message.

> The repository's `REQ-BHE-AUTH-002` (Get/List open to any authenticated caller because they sit on the availability hot path) is **unchanged**; only the MCP tool surface is owner-only (REQ-BHE-MCP-001).

#### Scenario: Range returns ordered rows

- GIVEN exceptions for `2026-12-24`, `2026-12-25` and `2026-12-26`
- WHEN an owner calls `list_hours_exceptions {from: "2026-12-24", to: "2026-12-26"}`
- THEN all three rows MUST be returned ordered by `exception_date` ascending

#### Scenario: Empty range is not an error

- GIVEN no exception in the requested range
- WHEN an owner calls `list_hours_exceptions` for that range
- THEN the response MUST be an empty array and MUST NOT be an error

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

An exception created through `create_hours_exception` MUST immediately govern `check_availability`, `create_booking` and `reschedule_booking`, because those paths already consume `BusinessHoursExceptionRepo.Get`. This change MUST NOT modify the availability service, the slot-context resolver or the booking use cases.

#### Scenario: Wire-created exception changes availability

- GIVEN the weekly `business_hours` says the business is open on a date
- WHEN an owner creates a closing exception (`is_closed = true`) for that date through the tool
- THEN `check_availability` for a slot on that date MUST report the business closed for the exception date
- AND deleting the exception MUST restore the weekly-schedule behavior

## Notes

- **Immutability is intentional and pre-existing.** The delete + create edit path is the repository's documented model (`internal/repository/business_hours_exception.go:19-22`), not a limitation of this change.
- **Business-wide by design.** The table has no `professional_id` (PRD §3.7.3); per-professional holidays are a future schema+product change, not covered here.
- **No national-holiday seed.** PRD §3.7.3 rejects an automatic holiday library; the owner loads the dates that matter (this is now possible through the tool).
- **`created_at` is hidden from the wire** (owner decision 2026-10-11, OQ-3): the tool output omits it, consistent with `serviceOut` and PRD §513's "sin timestamps" convention. The column remains in SQLite and in the repository row; only the wire shape drops it.
- **Tool count** grows 20 → 23 (owner decision 2026-10-11, OQ-1: three separate tools, one action each, per the ADR-0015 precedent).

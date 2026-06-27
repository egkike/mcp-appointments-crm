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

## Notes

- The unique index on `exception_date` is also the secondary index that makes the 3a step of `check_availability` an O(log n) lookup.
- This table deliberately does NOT auto-populate from a national holiday library. The owner is expected to add the dates that affect their business. ADR-0006 Decisión 2 documents the rejected alternatives.
- See `bookings` capability for how this table is consumed by the 5-step chain.

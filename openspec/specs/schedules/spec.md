# Spec: schedules

> Reference: `docs/PRD.md` §3.7.5; `docs/architecture/0004-naming-conventions.md`
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir el horario semanal de cada profesional. Una fila representa un día de la semana en que el profesional trabaja, con su hora de inicio y fin. Esta tabla es consultada por `check_availability` (Paso 3b) para responder "¿el Profesional A trabaja este día de la semana?".

## Requirements

### Requirement: One row per (professional, weekday)

The system MUST enforce that the combination `(professional_id, day_of_week)` is unique. A second schedule row for the same professional on the same weekday MUST fail at the database level.

#### Scenario: Inserting a duplicate weekday for the same professional fails

- GIVEN the table already has a row for `(professional_id = 'p-001', day_of_week = 1)`
- WHEN an insert with the same `(professional_id, day_of_week)` is attempted
- THEN the database MUST reject the statement with a unique-constraint violation

#### Scenario: Same weekday for a different professional is allowed

- GIVEN the table already has a row for `(professional_id = 'p-001', day_of_week = 1)`
- WHEN an insert for `(professional_id = 'p-002', day_of_week = 1)` is attempted
- THEN the insert MUST succeed

### Requirement: `day_of_week` range is 0 to 6

The `day_of_week` column MUST be an `INTEGER` in the inclusive range `0..6`, where `0` represents Sunday and `6` represents Saturday.

#### Scenario: Rejecting day_of_week outside the range

- GIVEN a fresh table
- WHEN an insert with `day_of_week = 7` is attempted
- THEN the application-level validation MUST reject the input with the semantic error `Error: day_of_week debe estar entre 0 (Domingo) y 6 (Sábado)`

#### Scenario: Rejecting negative day_of_week

- GIVEN a fresh table
- WHEN an insert with `day_of_week = -1` is attempted
- THEN the application-level validation MUST reject the input with the same range error

### Requirement: `start_time` and `end_time` in HH:MM

The `start_time` and `end_time` columns MUST be `TEXT` values in the `HH:MM` 24-hour format, expressed in the business timezone (which lives on `business_profile.timezone`). `start_time` MUST be strictly earlier than `end_time`.

#### Scenario: Valid range persisted

- GIVEN a fresh table
- WHEN a row is inserted with `start_time = '09:00'` and `end_time = '18:00'`
- THEN the row MUST be stored verbatim and a subsequent SELECT MUST return those exact values

#### Scenario: Rejecting malformed time

- GIVEN a fresh table
- WHEN a row is inserted with `start_time = '9:00 AM'`
- THEN the application-level validation MUST reject the input with a semantic error indicating the expected format

#### Scenario: start_time not earlier than end_time

- GIVEN a fresh table
- WHEN a row is inserted with `start_time = '18:00'` and `end_time = '09:00'`
- THEN the application-level validation MUST reject the input with a semantic error indicating that the start time is not before the end time

### Requirement: Missing row means the professional does not work that day

If no row exists for a given `(professional_id, day_of_week)` pair, the system MUST treat the professional as not working on that weekday.

#### Scenario: No schedule for Wednesday

- GIVEN a professional with schedules on Monday and Tuesday only
- WHEN `check_availability` is called for a Wednesday slot with that professional
- THEN the system MUST return a semantic error indicating the professional does not work on Wednesdays

### Requirement: Cascade delete from professionals

Deleting a `professionals` row (which is not exposed by the public repository contract but may occur through direct SQL during maintenance) MUST cascade to delete all `schedules` rows that reference that professional.

#### Scenario: Deleting a professional removes their schedules

- GIVEN a professional with three schedule rows
- WHEN the professional row is deleted
- THEN the database MUST also delete all three schedule rows that referenced it

#### Scenario: Application-level delete not exposed

- GIVEN the repository contract
- WHEN the contract is consulted
- THEN there MUST NOT be a public method that hard-deletes a `professionals` row; soft delete via `status` is the only sanctioned way (see `professionals` capability)

## Notes

- The `schedules` table is a hard requirement for `check_availability` Paso 3b. Without it, the system cannot answer "¿el profesional trabaja este día?".
- Times are stored in the business timezone by convention; no conversion happens at the storage layer. Conversion is the responsibility of the application code that derives `day_of_week` and `HH:MM` strings from a `start_datetime` value.
- See `bookings` capability for the consumer of this table in the 5-step chain.
- See `professionals` capability for the cascade-delete semantics.

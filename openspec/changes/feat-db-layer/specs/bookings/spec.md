# Spec: bookings

> Reference: `docs/PRD.md` §3.7.8, §3.7.13; `docs/architecture/0004-naming-conventions.md`; `docs/architecture/0006-data-model-and-reservations.md` Decisiones 3 y 5
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir las reservas de servicios. Cada reserva referencia un cliente, un profesional y un servicio, con un inicio y fin calculados. La reserva lleva una máquina de estados explícita (`pending`, `confirmed`, `cancelled`) y la pieza central es la cadena `check_availability` de 5 pasos (PRD §3.7.13), que valida que un slot propuesto sea legal antes de crear la reserva. La tabla MUST llamarse `bookings` (no `appointments`, per ADR-0004).

## Requirements

### Requirement: Table is named `bookings`

The reservation table MUST be named `bookings`. The legacy name `appointments` MUST NOT appear in the canonical schema. Renaming is part of this capability (per ADR-0004).

#### Scenario: Schema contains `bookings`, not `appointments`

- GIVEN the schema initialization runs against a fresh database
- WHEN a `SELECT name FROM sqlite_master WHERE type = 'table'` is executed
- THEN the result MUST include `bookings` and MUST NOT include `appointments`

### Requirement: Foreign keys to clients, professionals, and services

The `bookings` table MUST have foreign key columns `client_id`, `professional_id` and `service_id` referencing `clients.id`, `professionals.id` and `services.id` respectively. An attempt to insert a booking with a non-existent value in any of those columns MUST fail with a foreign-key violation.

#### Scenario: Insert with non-existent client_id fails

- GIVEN no client with ID `c-bogus` exists
- WHEN a booking is inserted with `client_id = 'c-bogus'`
- THEN the database MUST reject the statement with a foreign-key violation, and the repository MUST surface that as a semantic error

#### Scenario: Insert with non-existent professional_id fails

- GIVEN no professional with ID `p-bogus` exists
- WHEN a booking is inserted with `professional_id = 'p-bogus'`
- THEN the database MUST reject the statement with a foreign-key violation, and the repository MUST surface that as a semantic error

#### Scenario: Insert with non-existent service_id fails

- GIVEN no service with ID `s-bogus` exists
- WHEN a booking is inserted with `service_id = 's-bogus'`
- THEN the database MUST reject the statement with a foreign-key violation, and the repository MUST surface that as a semantic error

### Requirement: `end_datetime` is denormalized

The `end_datetime` column MUST be stored explicitly and MUST equal `start_datetime + service.duration_minutes` at the time of insert or reschedule. The overlap check (Paso 3d) MUST use this stored column directly, without JOINing to `services` to compute the end on the fly.

#### Scenario: `end_datetime` computed at insert

- GIVEN a service with `duration_minutes = 30` and a `start_datetime = 2026-07-13T10:00:00-03:00`
- WHEN the repository creates a booking
- THEN the stored `end_datetime` MUST be `2026-07-13T10:30:00-03:00` (or its ISO 8601 equivalent with the same instant)

#### Scenario: Reschedule recomputes `end_datetime`

- GIVEN an existing booking with `start_datetime = 2026-07-13T10:00:00-03:00` and `end_datetime = 2026-07-13T10:30:00-03:00`
- WHEN the booking is rescheduled to `start_datetime = 2026-07-13T11:00:00-03:00` (same service, 30 minutes)
- THEN the stored `end_datetime` MUST be `2026-07-13T11:30:00-03:00`; the old `end_datetime` MUST NOT survive

#### Scenario: Overlap check uses stored `end_datetime`

- GIVEN a booking exists for `professional_id = 'p-001'` from `10:00` to `10:30`
- WHEN `check_availability` is called for `professional_id = 'p-001'`, `start = 10:15`, `duration = 30`
- THEN the system MUST detect the overlap and return a semantic error, without JOINing `services` to compute the existing booking's end

### Requirement: Status finite state machine

The `status` column MUST be one of `pending`, `confirmed`, `cancelled`. Transitions are restricted: `pending → confirmed`, `pending → cancelled`, and `confirmed → cancelled` are allowed; any other transition (including `cancelled → confirmed` and `cancelled → pending`) MUST fail with a semantic error.

#### Scenario: `pending → confirmed` is allowed

- GIVEN a booking with `status = 'pending'`
- WHEN the repository is asked to confirm the booking
- THEN the stored `status` MUST become `confirmed` and the call MUST NOT return an error

#### Scenario: `confirmed → cancelled` is allowed

- GIVEN a booking with `status = 'confirmed'`
- WHEN the repository is asked to cancel the booking
- THEN the stored `status` MUST become `cancelled` and the call MUST NOT return an error

#### Scenario: `cancelled → confirmed` is rejected

- GIVEN a booking with `status = 'cancelled'`
- WHEN the repository is asked to confirm the booking
- THEN the call MUST return a semantic error indicating that the transition is not allowed

#### Scenario: Unknown status value is rejected

- GIVEN a fresh table
- WHEN a booking is inserted with `status = 'unknown'`
- THEN the application-level validation MUST reject the input with a semantic error listing the valid values

### Requirement: `payment_method` is free text

The `payment_method` column MUST be a `TEXT` value identifying the payment method the client chose for the appointment (for example `efectivo`, `tarjeta`, `transferencia`). It MAY be `NULL` if not yet specified. The valid values are not enforced by the schema; the business is expected to use values consistent with `business_profile.accepted_payment_methods`.

#### Scenario: Payment method stored

- GIVEN a fresh table
- WHEN a booking is inserted with `payment_method = 'efectivo'`
- THEN a subsequent SELECT MUST return that exact value

#### Scenario: Payment method omitted is allowed

- GIVEN a fresh table
- WHEN a booking is inserted with `payment_method = NULL`
- THEN the insert MUST succeed

### Requirement: Cancellation does not delete the row

Cancelling a booking MUST set `status = 'cancelled'` on the existing row, NOT delete the row. Cancelled bookings MUST remain in the table so that historical reporting and the `bookings.end_datetime` history remain consistent.

#### Scenario: Cancel preserves the row

- GIVEN a booking exists with a known `id`
- WHEN the cancel operation succeeds
- THEN a SELECT by that `id` MUST still return the row, with `status = 'cancelled'` and the same `start_datetime` / `end_datetime` as before

#### Scenario: Cancelled booking does not block subsequent bookings

- GIVEN a cancelled booking from `10:00` to `10:30` for `professional_id = 'p-001'`
- WHEN `check_availability` is called for the same professional from `10:00` to `10:30`
- THEN the cancelled booking MUST NOT count as a conflict

### Requirement: `check_availability` is a 5-step validation chain

`CheckAvailability(ctx, params)` MUST execute the validations in the order documented in PRD §3.7.13 and return the first failure as a semantic Spanish error. The five validations are: (3a) ¿Negocio abierto ese día?; (3b) ¿Profesional trabaja ese día?; (3c) ¿Slot cabe en el horario?; (3d) ¿Overlap con otra reserva?; (3e) ¿Slot no en el pasado?.

#### Scenario: 3a — business closed by exception

- GIVEN a `business_hours_exception` row for the requested date with `is_closed = 1` and `reason = 'Navidad'`
- WHEN `CheckAvailability` is called for a slot on that date
- THEN the method MUST return the error `Error: el negocio está cerrado el {fecha} ({reason}).` and MUST NOT proceed to step 3b

#### Scenario: 3a — business closed by JSON weekly schedule

- GIVEN no exception for the requested date
- AND the JSON `business_hours` has the corresponding weekday set to `null`
- WHEN `CheckAvailability` is called for a slot on that date
- THEN the method MUST return the error `Error: el negocio no abre los {día}.` and MUST NOT proceed to step 3b

#### Scenario: 3b — professional does not work that weekday

- GIVEN no exception and the business is open on the requested date
- AND the professional has no `schedules` row for that weekday
- WHEN `CheckAvailability` is called for a slot on that date
- THEN the method MUST return the error `Error: el Profesional {name} no trabaja los {día}.`

#### Scenario: 3c — slot ends after the closing time

- GIVEN the professional works `09:00` to `18:00` and the business is open `09:00` to `18:00`
- WHEN `CheckAvailability` is called for a `start_datetime` of `17:45` with a 30-minute service
- THEN the method MUST return the error `Error: el servicio dura 30 minutos pero solo quedan {remaining} antes del cierre a las 18:00.`

#### Scenario: 3c — slot before professional's start time (not business opening)

- GIVEN el profesional A tiene schedule `day_of_week=1, start_time=10:00, end_time=18:00` y el negocio abre a las 09:00
- WHEN se solicita un slot a las 09:30 con el profesional A
- THEN el sistema retorna `&SemanticError{Code: ErrCodeSlotOutOfHours, Message: "el Profesional A empieza a las 10:00."}` (no "el negocio abre a las 09:00")

#### Scenario: 3d — overlap with existing non-cancelled booking

- GIVEN a non-cancelled booking exists for the same professional that overlaps the proposed slot
- WHEN `CheckAvailability` is called
- THEN the method MUST return the error `Error: el Profesional {name} ya tiene una reserva de {existing_start} a {existing_end}.`

#### Scenario: 3d — overlap check ignores cancelled bookings

- GIVEN only a cancelled booking exists in the proposed slot
- WHEN `CheckAvailability` is called
- THEN the method MUST NOT flag the slot as a conflict

#### Scenario: 3e — slot in the past

- GIVEN the proposed `start_datetime` is before the current time
- WHEN `CheckAvailability` is called
- THEN the method MUST return the error `Error: no se puede reservar en el pasado.`

#### Scenario: Happy path — slot passes all five steps

- GIVEN a slot that satisfies 3a, 3b, 3c, 3d and 3e
- WHEN `CheckAvailability` is called
- THEN the method MUST return a `nil` error and a result that indicates the slot is available

#### Scenario: First failure wins

- GIVEN a slot that would fail both 3a (business closed) and 3d (overlap with another booking)
- WHEN `CheckAvailability` is called
- THEN the method MUST return the 3a error and MUST NOT execute 3d

### Requirement: `start_datetime` and `end_datetime` use ISO 8601 with timezone

The `start_datetime` and `end_datetime` columns MUST be `TEXT` values holding ISO 8601 datetimes with a timezone offset (for example `2026-07-13T10:00:00-03:00`). Storing without a timezone offset is not allowed because the business timezone is known and a booking must be unambiguous across DST transitions.

#### Scenario: Datetime with timezone offset persisted

- GIVEN a fresh table
- WHEN a booking is inserted with `start_datetime = '2026-07-13T10:00:00-03:00'`
- THEN a subsequent SELECT MUST return that exact string verbatim

## Notes

- The `end_datetime` denormalization is the foundation of the 3d overlap check: the SQL is a simple range comparison without JOIN. See ADR-0006 Decisión 3 for the rejected alternatives (JOIN on read, generated column, triggers that recompute on `service.duration_minutes` change).
- Cancelled bookings remain in the table for history but are excluded from the 3d overlap check via the `status != 'cancelled'` predicate.
- The full happy-path reservation flow includes `create_booking` (Paso 4) and the generation of a `confirmation_requested` alert (Paso 5), which is part of the `pending-alerts` capability.
- The 5-step chain is the single most-tested surface of Fase 1; the proposal recommends table-driven tests covering each step in isolation plus an end-to-end happy-path test.
- See `data-access` capability for the testing strategy (sqlmock per step + a small in-memory SQLite integration test for the chain as a whole).
- Cross-references: `professionals` (FK target, status filter), `schedules` (3b), `services` (FK target, duration for `end_datetime`), `business-profile` (3a JSON `business_hours`), `business-hours-exception` (3a exceptions), `pending-alerts` (Paso 5 alert creation).

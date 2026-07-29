# Spec: pending-alerts

> Reference: `docs/PRD.md` §3.7.10; §3.7.13 Paso 5
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe mantener una cola de notificaciones pull-based para que Hermes (u otro consumidor) pueda leer las alertas pendientes (`get_pending_alerts`) y marcarlas como enviadas (`mark_alert_as_sent`) cuando confirma con el cliente. Las alertas cubren tres tipos: pedido de confirmación, recordatorio 24 horas antes, y alerta de fidelización. La cola es opcionalmente enlazable a una reserva concreta vía `related_booking_id`.

## Requirements

### Requirement: Status finite state machine

The `status` column MUST be one of `pending`, `sent`, `cancelled`. Newly created alerts MUST default to `pending`. A cancelled alert MUST NOT be returned by `ListPending`.

#### Scenario: Default status is pending

- GIVEN a fresh table
- WHEN an alert is inserted without specifying `status`
- THEN the stored value MUST be `pending`

#### Scenario: Unknown status value is rejected

- GIVEN a fresh table
- WHEN an alert is inserted with `status = 'unknown'`
- THEN the application-level validation MUST reject the input with a semantic error listing the valid values

### Requirement: `scheduled_datetime` uses ISO 8601 UTC with millisecond precision

The `scheduled_datetime` column MUST be a `TEXT` value holding an ISO 8601 UTC
datetime with millisecond precision (regex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`),
indicating the moment at which the alert should be considered eligible for sending.
The repository converts any input timezone to UTC at insert time.

#### Scenario: UTC datetime stored verbatim

- GIVEN a fresh table
- WHEN an alert is inserted with `scheduled_datetime = '2026-07-12T16:00:00.000Z'`
- THEN a subsequent SELECT MUST return that exact string verbatim

### Requirement: `ListPending` returns due alerts only

`ListPending(ctx, limit, beforeTime)` MUST return alerts with `status = 'pending'` AND `scheduled_datetime <= beforeTime`, ordered by `scheduled_datetime ASC` (the oldest due alert first). Alerts that are scheduled in the future relative to `beforeTime` MUST NOT be returned.

#### Scenario: Returns due alerts in ascending order

- GIVEN three pending alerts with `scheduled_datetime` of `T+1h`, `T+30m` and `T+2h`, and `beforeTime = T+90m`
- WHEN `ListPending(ctx, 10, T+90m)` is called
- THEN the result MUST include exactly the two alerts scheduled at `T+30m` and `T+1h`, in that order

#### Scenario: Cancelled alerts are excluded

- GIVEN two alerts with the same `scheduled_datetime`, one with `status = 'pending'` and one with `status = 'cancelled'`
- WHEN `ListPending(ctx, 10, that_time)` is called
- THEN the result MUST include only the pending one

#### Scenario: Sent alerts are excluded

- GIVEN two alerts with the same `scheduled_datetime`, one with `status = 'pending'` and one with `status = 'sent'`
- WHEN `ListPending(ctx, 10, that_time)` is called
- THEN the result MUST include only the pending one

#### Scenario: Limit caps the result size

- GIVEN five due pending alerts exist
- WHEN `ListPending(ctx, 2, now)` is called
- THEN the result MUST contain exactly two alerts (the two oldest by `scheduled_datetime`)

#### Scenario: No due alerts returns empty

- GIVEN no pending alert has `scheduled_datetime <= beforeTime`
- WHEN `ListPending(ctx, 10, beforeTime)` is called
- THEN the result MUST be an empty slice (not `nil` and not an error)

### Requirement: `MarkAsSent` transitions a pending alert to sent

`MarkAsSent(ctx, id)` MUST set `status = 'sent'` on the alert with that ID. The method MUST be idempotent: marking an already-sent alert is a no-op (no error).

#### Scenario: Pending alert is marked as sent

- GIVEN a pending alert with `id = 42`
- WHEN `MarkAsSent(ctx, 42)` is called
- THEN a subsequent SELECT MUST show `status = 'sent'` for that alert

#### Scenario: Already-sent alert is a no-op

- GIVEN a sent alert with `id = 42`
- WHEN `MarkAsSent(ctx, 42)` is called
- THEN the call MUST NOT return an error and the stored `status` MUST remain `sent`

#### Scenario: MarkAsSent on cancelled alert is a no-op

- GIVEN una alert con `status = 'cancelled'`
- WHEN `MarkAsSent(ctx, 42)` is called
- THEN the system MUST return `nil` (success, no-op)
- AND the stored `status` MUST remain `cancelled` (not modified)
- AND the caller receives a "all good" semantic without error

### Requirement: `Cancel` transitions an alert to cancelled

`Cancel(ctx, id)` MUST set `status = 'cancelled'` on the alert with that ID. If the alert is already `sent` or `cancelled`, the call MUST return `nil` (no-op, idempotent) without modifying the `status`.

#### Scenario: Cancel an alert

- GIVEN an existing alert with `status = 'pending'`
- WHEN `Cancel(ctx, id)` is called
- THEN the system sets `status = 'cancelled'`
- AND the call returns `nil` (success)

#### Scenario: Cancel a sent or already-cancelled alert is a no-op

- GIVEN an alert with `status = 'sent'` or `status = 'cancelled'`
- WHEN `Cancel(ctx, id)` is called
- THEN the system returns `nil` (no-op, idempotent)
- AND the `status` is not changed

### Requirement: `related_booking_id` is optional

The `related_booking_id` column MAY be `NULL` (for system-generated alerts not tied to a specific booking, e.g., a global loyalty summary). When present, it MUST reference an existing `bookings.id`. The foreign key constraint is what makes the relationship enforceable.

#### Scenario: Alert without a related booking

- GIVEN a fresh table
- WHEN an alert is inserted with `related_booking_id = NULL`
- THEN the insert MUST succeed

#### Scenario: Alert linked to a real booking

- GIVEN a booking with `id = 'b-001'` exists
- WHEN an alert is inserted with `related_booking_id = 'b-001'`
- THEN the insert MUST succeed and the foreign key MUST be satisfied

#### Scenario: Alert linked to a non-existent booking fails

- GIVEN no booking with `id = 'b-bogus'` exists
- WHEN an alert is inserted with `related_booking_id = 'b-bogus'`
- THEN the database MUST reject the statement with a foreign-key violation

### Requirement: `type` is a free-text discriminator

The `type` column MUST be a `TEXT` value identifying the kind of alert. The canonical values are `confirmation_requested`, `reminder_24h`, and `loyalty_alert`, but the column is not constrained to that set at the database level; the application validates against that allowlist.

### Requirement: Allowed alert types (Fase 1)

In Fase 1, the only supported `type` is `confirmation_requested` (sent at booking creation per §3.7.13 Paso 5). The other types (`reminder_24h`, `loyalty_alert`) are reserved for Fase 2+. If `Create` is called with a different `type`, it MUST return `&SemanticError{Code: ErrCodeInvalidInput, ...}`.

#### Scenario: Only `confirmation_requested` is accepted in Fase 1

- GIVEN a fresh table
- WHEN alerts are inserted with `type` of `confirmation_requested`, `reminder_24h` and `loyalty_alert`
- THEN only the `confirmation_requested` insert succeeds
- AND the `reminder_24h` and `loyalty_alert` inserts MUST return `&SemanticError{Code: ErrCodeInvalidInput, Message: "tipo de alerta 'X' no soportado en Fase 1; sólo 'confirmation_requested'."}`

#### Scenario: Unknown type is rejected at the application layer

- GIVEN a fresh table
- WHEN an alert is inserted with `type = 'unknown_kind'`
- THEN the application-level validation MUST reject the input with a semantic error listing the valid types

### Requirement: Secondary index on `(scheduled_datetime, status)`

The table MUST have an index on `(scheduled_datetime, status)` so that the `ListPending` query is index-served and scales to thousands of alerts without a full table scan.

#### Scenario: Index exists

- GIVEN the schema initialization runs against a fresh database
- WHEN a `PRAGMA index_list('pending_alerts')` is executed
- THEN the result MUST include the index named `idx_pending_alerts_scheduled_status` (or equivalent) on columns `(scheduled_datetime, status)`

## Notes

- The `confirmation_requested` alert is the alert generated by Paso 5 of the reservation flow (see `bookings` capability). Other types are pre-computed reminders and loyalty alerts that the MCP server enqueues on a schedule.
- Pull-based delivery (vs push) is the project's choice because the MCP server runs loopback-only and the upstream LLM (Hermes) is the only consumer. A push channel (e.g., WhatsApp API) would require credentials and an external service, both out of scope per ADR-0005.
- See `data-access` for the testing strategy.

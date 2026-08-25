# Delta for pending-alerts

> **Change**: feat-mcp-server-advanced · Fase 3 (RF7, approach B1) — lifecycle wiring + CancelByBookingID

## ADDED Requirements

### REQ-PA-LIFE-001 — Booking lifecycle drives alert lifecycle

After a booking mutation succeeds, booking use cases MUST drive `pending_alerts`: successful `create_booking` inserts exactly one `confirmation_requested` alert (`status='pending'`, `related_booking_id` = the new booking, `scheduled_datetime` = creation time, message per §3.7.13 Paso 5: `Confirmar reserva de {client_name} con {pro_name} el {start_datetime}`). Successful `cancel_booking` cancels the booking's pending alerts; successful `reschedule_booking` cancels them then inserts one new `confirmation_requested` for the new schedule. An alert-persistence failure after a successful mutation MUST NOT fail the booking: it MUST be logged and success returned.

#### Scenario: Create inserts one pending confirmation alert

- GIVEN no alerts exist for a new booking
- WHEN `create_booking` succeeds
- THEN exactly one `confirmation_requested` alert exists, `status='pending'`, with that `related_booking_id`

#### Scenario: Alert message follows the Paso 5 template

- GIVEN a booking for client `Juan Pérez` with professional `Ana` starting `2026-09-01T10:00:00Z`
- WHEN `create_booking` succeeds
- THEN the alert message is `Confirmar reserva de Juan Pérez con Ana el 2026-09-01T10:00:00Z` (names/start as stored, UTC)

#### Scenario: Cancel cancels the booking's pending alerts

- GIVEN a booking with a pending `confirmation_requested` alert
- WHEN `cancel_booking` on that booking succeeds
- THEN the alert's `status` is `cancelled`

#### Scenario: Cancel with no pending alerts is not an error

- GIVEN a booking whose alerts are all already `sent`
- WHEN `cancel_booking` on that booking succeeds
- THEN the call returns success and the sent alerts remain `sent`

#### Scenario: Reschedule cancels pending and inserts a new alert

- GIVEN a booking with a pending confirmation alert for the old schedule
- WHEN `reschedule_booking` to a new start succeeds
- THEN the old alert is `cancelled`
- AND exactly one new pending `confirmation_requested` alert exists for the new schedule

#### Scenario: Alert-save failure does not fail the booking

- GIVEN alert persistence fails after the booking mutation commits
- WHEN `create_booking` (or `reschedule_booking`) runs
- THEN the booking result is returned as success
- AND the alert failure is logged for the operator

### REQ-PA-CANCEL-002 — CancelByBookingID is pending-only and idempotent

`CancelByBookingID(ctx, bookingID)` MUST transition to `cancelled` only alerts with that `related_booking_id` AND `status='pending'`; alerts already `sent`/`cancelled` MUST NOT change. No matching pending alert → success, no error.

#### Scenario: Only pending alerts are cancelled

- GIVEN one booking with alerts in states `pending`, `sent` and `cancelled`
- WHEN `CancelByBookingID(ctx, bookingID)` is called
- THEN only the `pending` alert becomes `cancelled`; the others are unchanged

#### Scenario: No pending alerts returns success

- GIVEN a booking id with no pending alerts (or none at all)
- WHEN `CancelByBookingID(ctx, bookingID)` is called
- THEN the call returns `nil` without error

#### Scenario: Repeated cancellation is idempotent

- GIVEN `CancelByBookingID` already ran for a booking
- WHEN it is called again with the same booking id
- THEN the call returns `nil` and no alert state changes

## MODIFIED Requirements

### Requirement: Allowed alert types (Fase 1)

In Fase 1, the only supported `type` is `confirmation_requested` (booking creation, §3.7.13 Paso 5). The other types (`reminder_24h`, `loyalty_alert`) are reserved for Fase 2+. If `Create` is called with a different `type`, it MUST return `&SemanticError{Code: ErrCodeInvalidInput, ...}`.

In Fase 3 the allowlist REMAINS `confirmation_requested` only (user decisions 2–3): `reminder_24h`/`loyalty_alert` generation stays out of scope; loyalty needs are served manually via the loyalty report tool.

(Previously: Fase 1 allowlist silent on Fase 3; this change pins the allowlist to `confirmation_requested` through Fase 3.)

#### Scenario: Only `confirmation_requested` is accepted in Fase 1

- GIVEN a fresh table
- WHEN alerts are inserted with `type` of `confirmation_requested`, `reminder_24h` and `loyalty_alert`
- THEN only the `confirmation_requested` insert succeeds
- AND the `reminder_24h` and `loyalty_alert` inserts MUST return `&SemanticError{Code: ErrCodeInvalidInput, Message: "tipo de alerta 'X' no soportado en Fase 1; sólo 'confirmation_requested'."}`

#### Scenario: Unknown type is rejected at the application layer

- GIVEN a fresh table
- WHEN an alert is inserted with `type = 'unknown_kind'`
- THEN the application-level validation MUST reject the input with a semantic error listing the valid types

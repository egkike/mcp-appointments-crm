# Delta for bookings

> **Change**: feat-mcp-server-advanced · Fase 3 (RF8, approach C1) — client aggregation for the loyalty report.
> Coordination: report-facing behavior (period parsing, `top_n` validation, auth, PII) lives in the `loyalty-report` spec.

## ADDED Requirements

### REQ-BK-AGG-001 — AggregateByClient counts non-cancelled bookings per client

The aggregation over bookings MUST count bookings per `client_id` within an inclusive-lower, exclusive-upper UTC time window on `start_datetime`, joined to client identity (`client_id`, `name`, `phone`). Bookings with `status = 'cancelled'` MUST NOT be counted. Only clients with a positive count in the window MUST be returned, ordered by count DESC with a deterministic tie-break by client name ASC, capped by a caller-supplied limit.

#### Scenario: Aggregation counts only non-cancelled bookings

- GIVEN client `c-001` has two `confirmed` and one `cancelled` booking inside the window
- WHEN the aggregation is computed for that window
- THEN `c-001` is returned with `booking_count = 2`

#### Scenario: Window bounds are inclusive start, exclusive end

- GIVEN a booking with `start_datetime` exactly at the window start, and another exactly at the window end
- WHEN the aggregation is computed for that window
- THEN only the booking at the window start is counted

#### Scenario: Cancelled-only clients are excluded

- GIVEN client `c-002` has only cancelled bookings in the window
- WHEN the aggregation is computed
- THEN `c-002` MUST NOT appear in the result

#### Scenario: Ordering is count DESC then name ASC

- GIVEN two clients tie on `booking_count`
- WHEN the aggregation is computed
- THEN the tie is broken by client name ASC deterministically

#### Scenario: Limit caps the result

- GIVEN the aggregation matches more clients than the limit
- WHEN the aggregation is computed with that limit
- THEN the result contains at most the top `limit` clients by `booking_count`

# loyalty-report Specification

> **Change**: feat-mcp-server-advanced · Fase 3 (RF8)
> **Domain**: loyalty-report (NEW — no prior spec existed)

## Purpose

Expose `get_loyalty_report` to Hermes: the business's most frequent clients in a period, ranked by booking volume. Rows carry client PII (`name`, `phone`); access is restricted to trusted roles.

## Requirements

### REQ-LR-001 — Period is a closed enum

The `period` argument MUST accept exactly five values: `last_week`, `last_month`, `last_quarter`, `last_year`, `all_time`. Any other value MUST be rejected with `ErrInvalidInput` (Spanish semantic error listing valid values). Omitted `period` defaults to `last_month`. The window MUST be computed from the current UTC time (e.g. `last_week` = the 7 days preceding now UTC); `all_time` MUST have no lower bound.

#### Scenario: Valid period returns a report

- GIVEN clients with bookings in the last month
- WHEN `get_loyalty_report` is called with `period = 'last_month'`
- THEN a ranked result is returned (see REQ-LR-004)

#### Scenario: Invalid period is rejected

- GIVEN any caller
- WHEN `get_loyalty_report` is called with `period = 'yesterday'`
- THEN the response MUST be a semantic `ErrInvalidInput` error listing the five valid values

#### Scenario: Omitted period defaults to last_month

- GIVEN a caller omits `period`
- WHEN `get_loyalty_report` is called
- THEN the report MUST cover the `last_month` window

#### Scenario: Window starts from now UTC

- GIVEN the current time is `T` (UTC)
- WHEN the report is requested for `last_week`
- THEN only bookings with `start_datetime` within `[T − 7 days, T)` are counted

### REQ-LR-002 — top_n is clamped to [1, 100], default 10

The `top_n` argument MUST be clamped to the inclusive range [1, 100]: out-of-range values are clamped to the nearest bound, not rejected. Omitted `top_n` defaults to 10.

#### Scenario: top_n within range is honored

- GIVEN 15 clients qualify
- WHEN the report is requested with `top_n = 15`
- THEN exactly 15 rows are returned

#### Scenario: top_n above 100 is clamped

- WHEN the report is requested with `top_n = 1000000`
- THEN the request succeeds and the result contains at most 100 rows

#### Scenario: top_n below 1 is clamped

- WHEN the report is requested with `top_n = 0`
- THEN the request succeeds and the result contains exactly 1 row (if any client qualifies)

#### Scenario: Omitted top_n defaults to 10

- GIVEN 25 clients qualify
- WHEN the report is requested without `top_n`
- THEN the result contains exactly 10 rows (the top 10)

### REQ-LR-003 — Access is owner/admin only

The report MUST require role `owner` or `admin`; staff, client, and unauthenticated callers MUST be rejected. Rows expose client `phone` (PII); the tool description SHOULD document this PII surface.

#### Scenario: Owner gets the report

- GIVEN an owner caller and qualifying clients
- WHEN `get_loyalty_report` is called
- THEN the ranked report is returned with `client_id`, `name`, `phone`, `booking_count`

#### Scenario: Staff, client and unauthenticated callers rejected

- GIVEN staff, client, and caller-less contexts
- WHEN `get_loyalty_report` is called
- THEN each call is rejected with the role-based rejection, independent of data

### REQ-LR-004 — Ranking counts non-cancelled bookings, DESC

Results MUST be ordered by `booking_count` DESC (tie-break by name ASC) counting only non-cancelled bookings with `start_datetime` in the window. Each row MUST carry `{client_id, name, phone, booking_count}`.

#### Scenario: Ranking is by booking count descending

- GIVEN `c-001` has 5 non-cancelled bookings and `c-002` has 2 in the window
- WHEN the report is requested
- THEN `c-001` ranks above `c-002`

#### Scenario: Cancelled bookings are not counted

- GIVEN `c-002`'s only booking in the window is cancelled
- WHEN the report is requested
- THEN `c-002` does not appear in the result

#### Scenario: Empty result when no client qualifies

- GIVEN no non-cancelled bookings exist in the window
- WHEN the report is requested
- THEN the result MUST be an empty list, not an error

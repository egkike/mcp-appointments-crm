# Delta for business-hours-exception

> **Change**: feat-repository-auth-integration
> **Domain**: business-hours-exception — repository-layer auth wiring (PRD §3.8.4, §3.8.7 item 6, §3.7.13 Paso 3a, RF6)
> **Status**: Specified · 2026-08-23
>
> `admin` and `owner` are operationally equivalent (`auth-roles` spec); scenarios name one, the other behaves identically. "Caller-less context" = `context.Context` without a caller. **"Staff/client/unauth rejection"** means: `staff` and `client` callers get `domain.ErrForbidden`, a caller-less context gets `domain.ErrUnauthenticated`, and the database is not touched.

## ADDED Requirements

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

# Delta for clients

> **Change**: feat-repository-auth-integration · 2026-08-23
> **Domain**: clients — repository-layer auth wiring (PRD §3.8.4, §3.8.7 item 6, RF3, RF5)
>
> `admin`/`owner` are operationally equivalent (`auth-roles`). "Caller-less context" = ctx without a caller. **"Staff/client/unauth rejection"**: `staff`/`client` callers → `domain.ErrForbidden`; caller-less → `domain.ErrUnauthenticated`; DB untouched.

## ADDED Requirements

### REQ-CL-AUTH-001 — Writes are admin/owner-only

`Save`, `Create`, `Update`, `Delete` MUST require role `admin` or `owner` (RF5); otherwise staff/client/unauth rejection. Authorized semantics (`ErrInvalidInput`, `ErrConflict` on duplicate phone, `ErrNotFound`) unchanged.

#### Scenario: Admin write persists

- GIVEN an admin caller
- WHEN `Create(ctx, client)` is called with a valid client
- THEN the client MUST be persisted

#### Scenario: Staff, client, and unauthenticated writes rejected

- GIVEN staff, client, and caller-less contexts
- WHEN any of `Save`, `Create`, `Update`, `Delete` is called
- THEN staff/client/unauth rejection applies to each call

### REQ-CL-AUTH-002 — Phone lookup is admin/owner-only

`FindByPhone` MUST require role `admin` or `owner` — `phone` is `UNIQUE`, so an open lookup is a phone-enumeration oracle exposing other clients' `preferences` (PII). Otherwise staff/client/unauth rejection, regardless of phone existence.

#### Scenario: Admin finds a client by phone

- GIVEN an admin caller and a row with `phone = '+5491112345678'`
- WHEN `FindByPhone(ctx, '+5491112345678')` is called
- THEN the client MUST be returned

#### Scenario: Phone enumeration blocked

- GIVEN staff, client, and caller-less contexts
- WHEN each calls `FindByPhone` with any phone
- THEN staff/client/unauth rejection applies, independent of phone existence

### REQ-CL-AUTH-003 — FindByID is caller-scoped, no existence oracle

`admin`/`owner` get any row. A `client` caller gets only their own row; any other id collapses to `domain.ErrNotFound`, indistinguishable from a non-existent id (no oracle; PRD §3.8.4 bookings precedent). Otherwise staff/client/unauth rejection.

#### Scenario: Admin reads any client

- GIVEN an admin caller and rows for two clients
- WHEN `FindByID(ctx, <other client id>)` is called
- THEN that client MUST be returned

#### Scenario: Client reads own row

- GIVEN a client caller whose row id is `+5491112345678`
- WHEN `FindByID(ctx, '+5491112345678')` is called
- THEN their row MUST be returned, including `preferences`

#### Scenario: Cross-tenant read collapses to ErrNotFound

- GIVEN a client caller and an existing row of another client
- WHEN `FindByID(ctx, <other client id>)` is called
- THEN the error MUST be `domain.ErrNotFound`, identical to the error for a non-existent id

#### Scenario: Staff and unauthenticated reads rejected

- GIVEN staff and caller-less contexts
- WHEN `FindByID` is called
- THEN staff/client/unauth rejection applies

### REQ-CL-AUTH-004 — SearchFTS is caller-scoped, preserves ranking

`SearchFTS` MUST return full `bm25`-ranked results for `admin`/`owner` (RF3 unchanged). A `client` caller receives only their own row: matches on other clients' `name`/`preferences` MUST NOT be returned (PII); no own match yields an empty list, not an error (no oracle). Otherwise staff/client/unauth rejection.

#### Scenario: Admin gets ranked FTS results

- GIVEN an admin caller, two clients with `alergia` in `preferences`
- WHEN `SearchFTS(ctx, 'alergia')` is called
- THEN both are returned, ordered by `bm25` rank ASC

#### Scenario: Client search returns only own row

- GIVEN a client caller and another client, both matching `alergia`
- WHEN the client calls `SearchFTS(ctx, 'alergia')`
- THEN only their own row is returned

#### Scenario: Client search without own match returns empty

- GIVEN a client whose row doesn't match `alergia`, others that do
- WHEN the client calls `SearchFTS(ctx, 'alergia')`
- THEN the result is an empty list, no error

#### Scenario: Staff and unauthenticated search rejected

- GIVEN staff and caller-less contexts
- WHEN `SearchFTS` is called
- THEN staff/client/unauth rejection applies

### REQ-CL-AUTH-005 — GetOrCreate is own-phone-only for clients

`GetOrCreate` is unrestricted for `admin`/`owner` (RF5 idempotency unchanged). A `client` caller may only call it with the phone identifying their own row; any other phone → staff/client/unauth rejection regardless of existence (a client MUST NOT bind a foreign row to a phone).

#### Scenario: Client get-or-creates own phone

- GIVEN a client caller identified by `+5491112345678`, no row for that phone
- WHEN `GetOrCreate(ctx, '+5491112345678', 'Juan')` is called
- THEN the row MUST be created and returned
- AND a second call MUST return the existing row without overwriting the name

#### Scenario: Foreign phone blocked

- GIVEN client, staff, and caller-less contexts
- WHEN each calls `GetOrCreate` with a foreign phone
- THEN staff/client/unauth rejection applies, independent of phone existence

#### Scenario: Admin unrestricted

- GIVEN an admin caller
- WHEN `GetOrCreate(ctx, phone, name)` is called with any phone
- THEN the existing idempotent behavior MUST apply

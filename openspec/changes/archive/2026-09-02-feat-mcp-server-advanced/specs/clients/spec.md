# Delta for clients

> **Change**: feat-mcp-server-advanced · Fase 3 (RF3, user decision 1)
> **Domain**: clients — staff FTS scoping

## MODIFIED Requirements

### REQ-CL-AUTH-004 — SearchFTS is caller-scoped, preserves ranking

`SearchFTS` MUST return full `bm25`-ranked results for `admin`/`owner` (RF3 unchanged). A `client` caller receives only their own row: matches on other clients' `name`/`preferences` MUST NOT be returned (PII); no own match yields an empty list, not an error (no oracle). A `staff` caller receives only rows of clients linked to the staff member's own bookings (decision 1): results MUST be restricted to clients with at least one booking for the staff caller's professional identity, other matches MUST NOT be returned, and no linked match yields an empty list, not an error. Caller-less rejection applies.

(Previously: staff callers were forbidden from `SearchFTS`; this change replaces the blanket rejection with bookings-scoped results per user decision 1.)

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

#### Scenario: Staff search is bookings-scoped

- GIVEN a staff caller linked to professional `p-001`
- AND client `c-001` matches `alergia` and has a booking with `p-001`
- AND client `c-002` matches `alergia` but has no booking with `p-001`
- WHEN the staff caller calls `SearchFTS(ctx, 'alergia')`
- THEN only `c-001` is returned, ranked by `bm25` among the in-scope matches

#### Scenario: Staff search without linked match returns empty

- GIVEN a staff caller whose linked clients do not match the query, and other clients that do
- WHEN the staff caller calls `SearchFTS(ctx, 'alergia')`
- THEN the result is an empty list, no error

#### Scenario: Staff scoping includes bookings regardless of status

- GIVEN a staff caller linked to professional `p-001`
- AND client `c-003` matches the query and has only a cancelled booking with `p-001`
- WHEN the staff caller calls `SearchFTS(ctx, <query>)`
- THEN `c-003` is returned (the linkage predicate considers bookings in any status)

#### Scenario: Unauthenticated search rejected

- GIVEN a caller-less context
- WHEN `SearchFTS` is called
- THEN caller-less rejection applies

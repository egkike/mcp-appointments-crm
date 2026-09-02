# Spec: clients

> Reference: `docs/PRD.md` §3.7.7, §3.7.10; `docs/architecture/0004-naming-conventions.md`; `docs/architecture/0006-data-model-and-reservations.md` Decisión 4
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir la ficha del cliente: nombre, teléfono (que funciona como ID del chat de WhatsApp/Telegram), email opcional, preferencias en texto libre y timestamps. La tabla soporta búsqueda full-text sobre nombre y preferencias para que Hermes pueda encontrar clientes por palabras clave (por ejemplo, "alergia a penicilina") y mantiene la unicidad de `phone` para que `get_or_create_client` sea idempotente.

## Requirements

### Requirement: `phone` is unique

The `phone` column MUST have a `UNIQUE` constraint. Two clients MUST NOT share the same `phone` value. The `phone` value is treated as the chat identifier in WhatsApp/Telegram, which is why uniqueness is mandatory.

#### Scenario: Inserting a duplicate phone fails

- GIVEN the table already has a row with `phone = '+5491112345678'`
- WHEN a second client is inserted with the same `phone`
- THEN the database MUST reject the statement with a unique-constraint violation, and the repository MUST surface that as a semantic `ErrConflict` to the caller

#### Scenario: `get_or_create_client` is idempotent

- GIVEN a client already exists with `phone = '+5491112345678'`
- WHEN the repository method `GetOrCreate(ctx, phone, name)` is called with the same `phone`
- THEN the method MUST return the existing client row, MUST NOT create a new row, and MUST NOT change the existing `name` unless explicitly requested

### Requirement: No messenger fields on clients

The `clients` table MUST NOT have `messenger_platform` or `messenger_id` columns. Those columns live on `business_profile` (the business's bot identity), not on individual clients.

#### Scenario: Schema does not contain messenger columns

- GIVEN the canonical schema
- WHEN a `PRAGMA table_info(clients)` is executed
- THEN the result MUST NOT include `messenger_platform` or `messenger_id` columns

### Requirement: `preferences` is free text

The `preferences` column MUST be a `TEXT` column that holds free-form notes about the client (for example `alergia a penicilina`, `prefiere turno a la tarde`). It MAY be `NULL` or empty.

#### Scenario: Preferences stored

- GIVEN a fresh table
- WHEN a client is inserted with `preferences = 'alergia a penicilina'`
- THEN a subsequent SELECT MUST return that exact value

#### Scenario: Empty preferences allowed

- GIVEN a fresh table
- WHEN a client is inserted with `preferences = NULL`
- THEN the insert MUST succeed

### Requirement: FTS5 index mirrors the source table

A virtual table `clients_fts` MUST exist with `content='clients'` and `content_rowid='rowid'`. The FTS index MUST mirror the `name` and `preferences` columns of the source table.

#### Scenario: FTS table created

- GIVEN the schema initialization runs against a fresh database
- WHEN a SELECT against `sqlite_master` is executed
- THEN a row describing the `clients_fts` virtual table MUST be present

### Requirement: FTS sync via SQL triggers (not Go code)

The system MUST keep `clients_fts` synchronized with `clients` using SQL triggers on `AFTER INSERT`, `AFTER UPDATE`, and `AFTER DELETE` of the source table. The repository layer MUST NOT execute any manual insert/update/delete against `clients_fts`.

#### Scenario: Insert into clients creates a matching FTS row

- GIVEN an empty database
- WHEN a client with `name = 'Juan Pérez'` and `preferences = 'alergia a penicilina'` is inserted
- THEN a SELECT against `clients_fts` MUST return one row with the same `name` and `preferences`

#### Scenario: Update changes the FTS row

- GIVEN a client exists in both `clients` and `clients_fts`
- WHEN the client's `preferences` is updated
- THEN a SELECT against `clients_fts` MUST reflect the new value, not the old one

#### Scenario: Delete removes the FTS row

- GIVEN a client exists in both `clients` and `clients_fts`
- WHEN the client row is deleted
- THEN a SELECT against `clients_fts` MUST NOT return that row

#### Scenario: Repository never writes to FTS directly

- GIVEN the repository source code
- WHEN the implementation is reviewed
- THEN there MUST NOT be any SQL statement targeting `clients_fts` for `INSERT`, `UPDATE` or `DELETE`; sync is exclusively via the triggers

### Requirement: Search returns FTS-ranked results

The repository method `SearchFTS(ctx, query)` MUST return clients that match the FTS5 query string, ordered by FTS5 rank (most relevant first).

#### Scenario: Match on preferences field

- GIVEN a client whose `preferences` contains the word `alergia`
- WHEN `SearchFTS(ctx, 'alergia')` is called
- THEN the result MUST include that client, ordered by relevance

#### Scenario: Match on name

- GIVEN a client whose `name` is `Juan Pérez`
- WHEN `SearchFTS(ctx, 'Juan')` is called
- THEN the result MUST include that client

#### Scenario: Malformed FTS query is rejected

- GIVEN the search method is called with a query that contains unbalanced parentheses or quote characters
- WHEN the FTS5 parser would otherwise fail
- THEN the repository MUST either sanitize the input or return a semantic error, and MUST NOT propagate a raw SQLite syntax error to the caller

#### Scenario: `SearchFTS` is ordered by relevance

- GIVEN multiple clients match a query
- WHEN `SearchFTS("allergy")` is called
- THEN the results MUST be ordered by FTS5 `bm25` rank ASC (lower rank = more relevant)

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

## Notes

- Trigger naming follows the convention `clients_fts_ai`, `clients_fts_au`, `clients_fts_ad` (infix `_fts_` for consistency with the table name). Confirmed 2026-06-25.
- ADR-0004 documents the move of `messenger_*` from `clients` to `business_profile`. Any pre-Fase-1 schema that still has those columns on `clients` MUST be considered a bug.
- `phone` uniqueness is also the foundation of `get_or_create_client` (RF5) — without it, the MCP tool would not be idempotent.
- The trigger integration test in `internal/db/database_test.go` covers both `clients_fts` and `services_fts`. See `data-access` capability.

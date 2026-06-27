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

### Requirement: Application-level rejection of malformed date formats

The repository MUST validate that date inputs (such as `exception_date` in `business_hours_exception`) match the canonical `YYYY-MM-DD` format before passing them to the database. Non-canonical date strings MUST NOT be silently stored.

#### Scenario: Rejects malformed exception_date format

- GIVEN an input with `exception_date` not matching `YYYY-MM-DD` (e.g., `2026-12-25T00:00:00` or `25/12/2026`)
- WHEN the repository is called
- THEN the call MUST return an error with `Code == ErrCodeInvalidInput`
- AND the database MUST NOT receive the INSERT

## Notes

- Trigger naming follows the convention `clients_fts_ai`, `clients_fts_au`, `clients_fts_ad` (infix `_fts_` for consistency with the table name). Confirmed 2026-06-25.
- ADR-0004 documents the move of `messenger_*` from `clients` to `business_profile`. Any pre-Fase-1 schema that still has those columns on `clients` MUST be considered a bug.
- `phone` uniqueness is also the foundation of `get_or_create_client` (RF5) — without it, the MCP tool would not be idempotent.
- The trigger integration test in `internal/db/database_test.go` covers both `clients_fts` and `services_fts`. See `data-access` capability.

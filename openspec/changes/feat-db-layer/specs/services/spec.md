# Spec: services

> Reference: `docs/PRD.md` §3.7.6, §3.7.10; `docs/architecture/0004-naming-conventions.md`; `docs/architecture/0006-data-model-and-reservations.md` Decisión 4
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir el catálogo de servicios que ofrece el negocio. Cada servicio tiene un nombre, una descripción, una duración en minutos, un precio y un flag de activo. El catálogo es consultable vía FTS5 (full-text search) para que Hermes pueda buscar por nombre o descripción, y la búsqueda devuelve resultados ordenados por rank FTS5.

## Requirements

### Requirement: `duration_minutes` is the canonical duration field

The `duration_minutes` column MUST exist as `INTEGER` and MUST be strictly greater than zero. The legacy name `duration_mins` MUST NOT appear anywhere in the canonical schema.

#### Scenario: Rejecting zero or negative duration

- GIVEN a fresh table
- WHEN a service is inserted with `duration_minutes = 0`
- THEN the application-level validation MUST reject the input with a semantic error indicating the duration must be positive

- WHEN a service is inserted with `duration_minutes = -5`
- THEN the application-level validation MUST reject the input with the same semantic error

#### Scenario: Duration is used by the bookings flow

- GIVEN a service with `duration_minutes = 30` and a booking with `start_datetime = 2026-07-13T10:00:00-03:00`
- WHEN the booking is created through the repository
- THEN the stored `end_datetime` MUST equal `start_datetime + 30 minutes`, computed in the service-level logic that creates the booking

### Requirement: `is_active` drives visibility

The `is_active` column MUST be a boolean (`0` or `1`). The repository method `ListActive(ctx)` MUST return only rows with `is_active = 1`. Inactive services MUST NOT be returned by that method.

#### Scenario: Active services returned

- GIVEN the table has one row with `is_active = 1` and another with `is_active = 0`
- WHEN `ListActive(ctx)` is called
- THEN the result MUST include the active service and MUST NOT include the inactive one

#### Scenario: Default is active

- GIVEN a fresh table
- WHEN a service is inserted without specifying `is_active`
- THEN the stored value MUST be `1`

### Requirement: FTS5 index mirrors the source table

A virtual table `services_fts` MUST exist with `content='services'` and `content_rowid='rowid'`. The FTS index MUST mirror the `name` and `description` columns of the source table.

#### Scenario: FTS table created with external content

- GIVEN the schema initialization runs against a fresh database
- WHEN a SELECT against `sqlite_master` is executed
- THEN a row describing the `services_fts` virtual table MUST be present

### Requirement: FTS sync via SQL triggers (not Go code)

The system MUST keep `services_fts` synchronized with `services` using SQL triggers on `AFTER INSERT`, `AFTER UPDATE`, and `AFTER DELETE` of the source table. The repository layer MUST NOT execute any manual insert/update/delete against `services_fts`.

#### Scenario: Insert into services creates a matching FTS row

- GIVEN an empty database
- WHEN a service with `name = 'Corte de pelo'` and `description = 'Corte clásico'` is inserted
- THEN a SELECT against `services_fts` MUST return one row with the same `name` and `description`

#### Scenario: Update changes the FTS row

- GIVEN a service exists in both `services` and `services_fts`
- WHEN the service's `name` is updated to `Corte de pelo premium`
- THEN a SELECT against `services_fts` MUST reflect the new name, not the old one

#### Scenario: Delete removes the FTS row

- GIVEN a service exists in both `services` and `services_fts`
- WHEN the service row is deleted
- THEN a SELECT against `services_fts` MUST NOT return that row

#### Scenario: Repository never writes to FTS directly

- GIVEN the repository source code
- WHEN the implementation is reviewed
- THEN there MUST NOT be any SQL statement targeting `services_fts` for `INSERT`, `UPDATE` or `DELETE`; sync is exclusively via the triggers

### Requirement: Search returns FTS-ranked results

The repository method `SearchFTS(ctx, query)` MUST return services that match the FTS5 query string, ordered by FTS5 rank (most relevant first). The query parameter is a raw FTS5 expression; the repository MUST escape or reject input that contains FTS5 operators that would break the query syntax.

#### Scenario: Exact-name match returns the service

- GIVEN a service with `name = 'Corte de pelo'`
- WHEN `SearchFTS(ctx, 'Corte')` is called
- THEN the result MUST include that service, ordered first

#### Scenario: Match on description

- GIVEN a service whose `description` contains the word `alergia`
- WHEN `SearchFTS(ctx, 'alergia')` is called
- THEN the result MUST include that service, ordered by relevance

#### Scenario: Malformed FTS query is rejected

- GIVEN the search method is called with a query that contains unbalanced parentheses or quote characters
- WHEN the FTS5 parser would otherwise fail
- THEN the repository MUST either sanitize the input or return a semantic error, and MUST NOT propagate a raw SQLite syntax error to the caller

## Notes

- Trigger naming follows the convention `services_fts_ai`, `services_fts_au`, `services_fts_ad` (infix `_fts_` for consistency with the table name). Confirmed 2026-06-25; documented in PRD §3.7.10 and the proposal.
- The trigger integration test in `internal/db/database_test.go` is the only place where real in-memory SQLite is used (because `go-sqlmock` cannot simulate trigger side effects). All other repository tests use `go-sqlmock`. See `data-access` capability.
- See `bookings` capability for how `services.duration_minutes` is consumed to compute `end_datetime`.
- See `data-access` capability for the testing split (sqlmock vs real in-memory).

# Spec: professionals

> Reference: `docs/PRD.md` §3.7.4; `docs/architecture/0004-naming-conventions.md`
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir el staff que presta servicios. Cada profesional tiene un identificador UUID, un nombre, un rol o especialidad principal, un estado (activo o inactivo), datos de contacto opcionales y una lista de IDs de servicios que sabe ofrecer. Los profesionales inactivos no deben aparecer en la lista de staff activo ni en los resultados de `check_availability`.

## Requirements

### Requirement: UUID primary key

The `id` column MUST be a `TEXT` value holding a UUID v4 string, generated when the professional is created.

#### Scenario: ID is auto-assigned at creation

- GIVEN the repository method `CreateProfessional(ctx, p)` is called with a `Professional` struct whose `ID` is empty
- WHEN the method inserts the row
- THEN the database MUST store a non-empty UUID v4 string in the `id` column and the repository MUST return the struct with the assigned ID

#### Scenario: Inserting with a non-UUID value fails

- GIVEN a fresh table
- WHEN a row is inserted with `id = 'not-a-uuid'`
- THEN the application-level validation MUST reject the input with a semantic error indicating that the ID is not a valid UUID v4

### Requirement: Status drives visibility

The `status` column MUST be a `TEXT` value equal to either `active` or `inactive`. The repository method `GetActive(ctx)` MUST return only rows with `status = 'active'`. Inactive professionals MUST be excluded from any user-facing list or from `check_availability` results.

#### Scenario: Active professional returned by GetActive

- GIVEN the table has one row with `status = 'active'` and another with `status = 'inactive'`
- WHEN `GetActive(ctx)` is called
- THEN the result MUST include the active professional and MUST NOT include the inactive one

#### Scenario: Inactive professional excluded from check_availability

- GIVEN a professional with `status = 'inactive'` and a schedule on Monday
- WHEN `check_availability` is called for a Monday slot with that professional's ID
- THEN the system MUST return a semantic error indicating the professional is not available

#### Scenario: Default status is active

- GIVEN a fresh table
- WHEN a professional is inserted without specifying `status`
- THEN the stored value MUST be `active`

### Requirement: `specialties` is a JSON array of service IDs

The `specialties` column MUST be a `TEXT` column that stores a JSON array of strings, where each string is a UUID v4 reference to an existing `services.id`. An empty array (or `NULL`) is allowed and means the professional is not yet mapped to any service.

#### Scenario: Multiple specialties persisted

- GIVEN a professional and two existing services with IDs `svc-001` and `svc-003`
- WHEN the specialties list `["svc-001","svc-003"]` is saved on the professional
- THEN a subsequent SELECT MUST return that exact JSON array

#### Scenario: Specialty referencing a non-existent service is rejected

- GIVEN no service with ID `svc-999` exists
- WHEN a professional is updated with `specialties = ["svc-999"]`
- THEN the repository MUST reject the update with a semantic error indicating the service is unknown

### Requirement: `role_specialty` is free text

The `role_specialty` column MUST be a `TEXT` value describing the main role of the professional (for example `Veterinario`, `Barbero`, `Estilista`). It MAY be `NULL` if the business does not need to differentiate roles.

#### Scenario: Role stored

- GIVEN a fresh table
- WHEN a professional is inserted with `role_specialty = 'Barbero'`
- THEN a subsequent SELECT MUST return that exact value for that row

### Requirement: Soft delete via `status` only

The system MUST NOT support hard deletion of a professional through the public repository methods. Setting `status = 'inactive'` is the only sanctioned way to remove a professional from active use; the row MUST remain in the table for historical reference (e.g., past `bookings` references).

#### Scenario: Inactive professional still references existing bookings

- GIVEN a professional who has past bookings
- WHEN the professional's `status` is set to `inactive`
- THEN the bookings rows MUST still be readable and MUST still reference the professional's UUID

#### Scenario: No hard delete method exists

- GIVEN the repository contract
- WHEN the contract is consulted
- THEN there MUST NOT be a `DeleteProfessional` method that removes the row; only an update of `status` is exposed

## Notes

- The `professionals` table was absent in some early docs (formalized in PRD §3.7.4 after the 2026-06-25 review). `internal/db/database.go` currently has no `professionals` table; this is one of the gaps fixed by Fase 1.
- The `specialties` JSON array is the de-normalized complement to the `schedules` table: a professional works certain days (`schedules`) and offers certain services (`specialties`).
- See `bookings` for how `professional_id` is consumed by the 5-step chain.
- See `data-access` for the testing strategy.

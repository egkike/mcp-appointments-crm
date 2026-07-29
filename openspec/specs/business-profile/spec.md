# Spec: business-profile

> Reference: `docs/PRD.md` §3.7.1, §3.7.2; `docs/architecture/0006-data-model-and-reservations.md` Decisión 1
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir la configuración global del negocio (perfil del local, datos de contacto, horario semanal regular, métodos de pago aceptados, identificador del canal de mensajería y la zona horaria) en una fila única por instalación. Esta fila es la fuente de verdad para que `check_availability` resuelva la apertura semanal, la currency y la `timezone`, y para que las herramientas MCP expongan la identidad del negocio a Hermes.

## Requirements

### Requirement: Singleton row constraint

The system MUST guarantee that the `business_profile` table contains at most one row per database, identified by a fixed primary key value of `singleton`.

#### Scenario: Fresh install — first call seeds the row

- GIVEN a database that has just been initialized and has no row in `business_profile`
- WHEN the repository method `GetBusinessProfile(ctx)` is invoked
- THEN the system MUST insert a row with `id = 'singleton'` and a non-null `name` (empty string is acceptable as a placeholder), and MUST return that row

#### Scenario: Re-invocation does not create a second row

- GIVEN a database that already has the singleton row
- WHEN `GetBusinessProfile(ctx)` is invoked again
- THEN the system MUST NOT insert a second row, and MUST return the existing row unchanged

#### Scenario: Direct INSERT of a second row fails (constraint violation)

- GIVEN a `business_profile` row with `id='singleton'` already exists
- WHEN a direct SQL `INSERT INTO business_profile (id, name) VALUES ('singleton', 'Other')` is attempted
- THEN the database MUST reject the statement with a constraint violation (PRIMARY KEY uniqueness on `id`)

#### Scenario: Direct INSERT with a different id is rejected by CHECK

- GIVEN no row exists in `business_profile`
- WHEN a direct SQL `INSERT INTO business_profile (id, name) VALUES ('something-else', 'X')` is attempted
- THEN the database MUST reject the statement with a CHECK constraint violation (`CHECK (id = 'singleton')` fails)
- AND the repository's `GetBusinessProfile` MUST still succeed because it uses `INSERT OR IGNORE ... VALUES ('singleton', ...)`

### Requirement: Lazy-init semantics for first-boot access

The repository method `GetBusinessProfile(ctx)` MUST be idempotent and self-healing: it MUST attempt to ensure the singleton row exists before reading it, so that callers never observe an empty result set on a fresh install.

#### Scenario: Two simultaneous first calls

- GIVEN a fresh database with no row
- WHEN two goroutines invoke `GetBusinessProfile(ctx)` concurrently
- THEN the system MUST end with exactly one row in the table, and BOTH calls MUST return that same row (no panics, no duplicate insert errors propagated to the caller)

### Requirement: Weekly schedule stored as JSON

The `business_hours` column MUST be a `TEXT` column that stores a JSON object with one entry per weekday. Days when the business is closed MUST be represented by the literal `null`. Open days MUST be objects with `open` and `close` keys holding values in `HH:MM` 24-hour format, expressed in the business timezone.

#### Scenario: Valid weekly schedule parses without error

- GIVEN a JSON object with entries for `monday` through `sunday`, where `sunday` is `null` and the other days are objects with `open`/`close` in `HH:MM`
- WHEN the repository writes that value into the `business_hours` column
- THEN the write MUST succeed and a subsequent `SELECT` MUST return the exact same JSON string

#### Scenario: Closed day represented as null

- GIVEN a business that does not operate on Sundays
- WHEN the owner saves the weekly schedule
- THEN the value for `sunday` in the stored JSON MUST be the literal `null`, not an empty string and not an object with `null` open/close

### Requirement: Accepted payment methods as JSON array

The `accepted_payment_methods` column MUST store a JSON array of strings, where each string identifies one payment method accepted by the business (for example `efectivo`, `tarjeta`, `transferencia`).

### Requirement: `accepted_payment_methods` is NULL or JSON array

The `accepted_payment_methods` column is TEXT, holding either NULL (means "no payment methods registered yet") or a JSON array of strings (e.g., `["efectivo","tarjeta","transferencia"]`). `Create`/`Update` MUST accept NULL as "empty". The repository MUST validate each entry is a non-empty string if the array is non-NULL.

#### Scenario: Multiple payment methods persisted

- GIVEN a business that accepts cash, card and bank transfer
- WHEN the owner saves the payment methods list
- THEN the stored column MUST be a valid JSON array containing exactly those three string values, in the order provided

#### Scenario: Empty payment methods list is allowed

- GIVEN a business that has not yet configured payment methods
- WHEN the column is empty or `NULL`
- THEN the system MUST NOT fail validation; the column is optional in the sense that absence is acceptable at first boot

### Requirement: Default configuration values

On a fresh install, the system MUST populate the singleton row with the following defaults: `currency_code = 'ARS'`, `currency_symbol = '$'`, `slot_interval_minutes = 30`, `timezone = 'UTC'`, `business_hours = '{}'`, and `accepted_payment_methods` empty.

#### Scenario: Defaults applied on first insert

- GIVEN a fresh database
- WHEN the singleton row is first inserted by the lazy-init flow
- THEN the values of `currency_code`, `currency_symbol`, `slot_interval_minutes` and `timezone` MUST match the defaults listed above unless an explicit value was provided

### Requirement: Messenger fields belong here, not on clients

The `messenger_platform` and `messenger_id` columns MUST exist on `business_profile` and MUST NOT exist on the `clients` table. The `messenger_platform` value MUST be one of `whatsapp`, `telegram`, or `NULL`.

#### Scenario: Messenger fields present on business_profile

- GIVEN the canonical schema
- WHEN a SELECT against `business_profile` is executed
- THEN the result MUST include `messenger_platform` and `messenger_id` columns

#### Scenario: Messenger fields absent on clients

- GIVEN the canonical schema
- WHEN a SELECT against `clients` is executed
- THEN the result MUST NOT include `messenger_platform` or `messenger_id` columns

### Requirement: `messenger_platform` CHECK constraint

The `business_profile` schema MUST include a CHECK constraint on `messenger_platform`:
`CHECK (messenger_platform IS NULL OR messenger_platform IN ('whatsapp', 'telegram'))`.

This enforces the allowlist at the DB level (defense in depth); the repository also validates at the app level.

#### Scenario: Invalid messenger_platform rejected by CHECK

- GIVEN a fresh `business_profile` table
- WHEN an INSERT or UPDATE sets `messenger_platform = 'sms'`
- THEN the database MUST reject the statement with a CHECK constraint violation

## Notes

- ADR-0004 documents the move of `messenger_*` from `clients` to `business_profile`. Any pre-Fase-1 schema that has those columns on `clients` MUST be considered a bug.
- The lazy-init flow uses `INSERT OR IGNORE` + `SELECT` per the proposal's third confirmed decision (2026-06-25). It is a repository-level concern; direct SQL access from outside the repository is not protected by this contract.
- See `bookings` capability for how `business_hours` and `timezone` are consumed by the 5-step `check_availability` chain (Paso 3a).
- See `data-access` capability for the testing strategy that covers this row.

### Requirement: Fresh install with empty business_hours rejects all bookings

The default value of `business_hours` is `'{}'` (empty JSON object). Until the config-wizard sets actual operating hours, `check_availability` MUST reject all booking attempts because no weekday has defined opening hours.

#### Scenario: Fresh install rejects all bookings until wizard sets hours

- GIVEN una base recién creada con `business_hours = '{}'`
- WHEN `CheckAvailability` se invoca con cualquier día
- THEN el sistema retorna `&SemanticError{Code: ErrCodeBusinessClosed, Message: "el negocio no abre los {día}."}`
- AND el operador debe correr el config-wizard o actualizar `business_hours` antes de aceptar bookings

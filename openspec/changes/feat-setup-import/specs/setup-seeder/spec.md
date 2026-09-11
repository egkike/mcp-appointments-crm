# Spec: setup-seeder

> Reference: `openspec/changes/feat-setup-import/proposal.md` Scope 4–8, D1–D6; `openspec/changes/feat-setup-import/exploration.md` §2–§5, §7; Issue #71
> Change: feat-setup-import
> Status: NEW (no prior spec existed)

## Purpose

On the first boot after installation, the server must seed the database (business profile, professionals, schedules, services) from the wizard setup JSONs in a single atomic SQLite transaction, guarded by the fresh signal `business_profile.name == ''`. After this seed, no re-seed ever happens, so runtime edits made through the MCP tools are safe. A fresh database with missing or malformed wizard output must fail fast instead of serving a silently half-configured CRM.

## Requirements

### Requirement: Fresh guard on business_profile.name

The seeder MUST execute only when the singleton `business_profile` row exists with `name == ''` (the lazy-init placeholder). The guard MUST NOT inspect the `professionals` or `services` tables. When `name != ''`, the loader and seeder MUST be skipped entirely and the server MUST continue booting normally, logging that the seed was skipped.

#### Scenario: Fresh database seeds

- GIVEN a database whose `business_profile` singleton row has `name == ''`
- WHEN the server boots
- THEN the seeder MUST run and seed the database from the setup JSONs

#### Scenario: Second boot does not re-seed

- GIVEN a database that was already seeded (the singleton row has a non-empty `name`)
- WHEN the server boots again
- THEN the seeder MUST NOT run and the server MUST boot normally

#### Scenario: Manual deletions are preserved

- GIVEN a seeded database where an operator later deleted all professionals but left the business profile intact (`name != ''`)
- WHEN the server boots
- THEN the seeder MUST NOT run; the deleted professionals MUST NOT be re-inserted

### Requirement: Single atomic transaction

The seed MUST execute inside exactly one SQLite transaction in this order: `BEGIN` → UPSERT the `business_profile` singleton row (all 18 wizard fields) → insert professionals (ID generated via `idgen.NewUUID`) → insert their schedules → insert services (ID generated via `idgen.NewUUID`) → `COMMIT`. If any step fails, the seeder MUST `ROLLBACK` the transaction and treat the failure as fatal; the database MUST be left in its exact pre-seed state (no partial rows, singleton placeholder row intact with `name == ''`).

#### Scenario: Happy path commits everything

- GIVEN a fresh database and the three valid setup JSONs
- WHEN the seeder runs
- THEN it MUST commit the business profile, every professional, every schedule, and every service in one transaction, and log a success message

#### Scenario: Mid-seed failure rolls back all writes

- GIVEN a fresh database and a seed that fails while inserting services (after professionals and schedules were inserted)
- WHEN the seeder runs
- THEN the transaction MUST be rolled back and the database MUST contain no seeded professionals, schedules, or services, and the singleton row MUST still have `name == ''`

### Requirement: Direct parameterized SQL, not repositories

The seeder MUST execute direct SQL statements within the transaction using `?` placeholders for every value; it MUST NOT call the RBAC-gated repository methods (repositories expose no transaction API — proposal D3). Entity `Validate()` calls SHOULD be retained as defense-in-depth where the entity shape matches the wizard shape (`BusinessProfile.Validate`, `Professional.Validate`). The seeder MUST NOT reject services with `price == 0` (proposal D5: the wizard allows `price >= 0` and the schema CHECK allows `price >= 0`; the `Service.Validate` `price > 0` mismatch is a documented follow-up, out of scope).

#### Scenario: Every seed statement uses placeholders

- GIVEN the seeder source code
- WHEN the SQL statements are reviewed
- THEN every statement MUST bind values via `?` placeholders and MUST NOT concatenate values into SQL strings

#### Scenario: Zero-price service seeds successfully

- GIVEN a `setup_services.json` containing a service with `price: 0`
- WHEN the seeder runs on a fresh database
- THEN that service MUST be inserted with `price = 0` and the seed MUST succeed

#### Scenario: Direct inserts keep FTS5 in sync

- GIVEN the `services_fts` FTS5 virtual table with its `AFTER INSERT` triggers
- WHEN the seeder inserts services via direct SQL
- THEN each inserted service MUST be findable through `services_fts` search

### Requirement: business_profile singleton upsert with all wizard fields

The seeder MUST update the existing singleton row (`id = 'singleton'`) in place with all 18 wizard field values; it MUST NOT insert a second row. The stored `business_hours` value MUST be the mapped numeric-string JSON (`"1"`..`"7"`, closed days absent — see `setup-loader`).

#### Scenario: Seeded profile is readable through the repository

- GIVEN a completed first-boot seed
- WHEN `GetBusinessProfile(ctx)` is invoked
- THEN the returned profile MUST carry the wizard values for all 18 fields, with `id = 'singleton'` and exactly one row in the table

#### Scenario: Seeded hours drive availability correctly

- GIVEN a completed first-boot seed where the wizard closed Sundays and opened Mondays 09:00–18:00
- WHEN `IsOpenOn` is evaluated for Monday and for Sunday
- THEN Monday MUST report open 09:00–18:00 and Sunday MUST report closed

### Requirement: Boot-time failure policy

When the database is fresh and any setup JSON is missing or malformed, the server MUST abort startup with a fatal error (exit code 1) whose message is a semantic Spanish string naming the offending file, and the database MUST remain in its pre-seed state. When the database is not fresh (`name != ''`), loader errors MUST be ignored and logged, and the server MUST boot normally.

#### Scenario: Fresh database with malformed JSON exits 1

- GIVEN a fresh database and a `setup_business.json` with invalid JSON
- WHEN the server boots
- THEN the process MUST exit with code 1, the error message MUST name `setup_business.json` in Spanish, and no seed rows MAY exist

#### Scenario: Fresh database with missing JSON exits 1

- GIVEN a fresh database and a setup directory missing `setup_staff.json`
- WHEN the server boots
- THEN the process MUST exit with code 1 and the error message MUST name the missing file

#### Scenario: Non-fresh database ignores bad JSON and boots

- GIVEN a seeded database (`name != ''`) and a setup directory with malformed JSON
- WHEN the server boots
- THEN the server MUST start and serve normally, logging that the setup import was skipped

### Requirement: Boot wiring in the composition root

The fresh-guard check and the seed MUST run in `cmd/mcp-server` immediately after `db.NewDatabase` returns and before repository construction, use-case wiring, and HTTP/SSE serving, so the seed executes in a single-threaded boot window.

#### Scenario: Seed runs before serving

- GIVEN a first boot on a fresh database with valid setup JSONs
- WHEN `main.run()` executes
- THEN the guard and seed MUST complete before the server begins listening on `127.0.0.1:3000`

#### Scenario: Rollback restores pre-hook behavior

- GIVEN the boot wiring commit is reverted
- WHEN the server boots
- THEN the server MUST behave exactly as before this change (no seed, empty profile, manual SQL seeding)

### Requirement: First-boot seeding is documented

`docs/installation.md` MUST state that the server seeds the database automatically on first boot when the business profile is empty, replacing the current wording that presents the JSONs as final artifacts with no consumer.

#### Scenario: Installation docs describe first-boot seeding

- GIVEN `docs/installation.md` after this change
- WHEN a reader searches for the setup JSON step
- THEN the document MUST explain that the server siembra la DB en el primer arranque si el perfil está vacío, and MUST NOT claim the JSONs are merely final artifacts

## Notes

- The fresh signal relies on the lazy-init `INSERT OR IGNORE` placeholder (`name == ''`); the wizard guarantees a non-empty name, so a seeded profile can never look fresh again (proposal D2).
- Boot concurrency is a non-issue by construction: the seed runs before HTTP serving, and the DSN already sets WAL + `busy_timeout=5000` (exploration R3).
- Setup JSONs are never modified or deleted by the server (proposal D6; enforced in the `setup-loader` capability).
- **Canonical drift warning:** `openspec/specs/business-profile/spec.md` requirement "Weekly schedule stored as JSON" still pins day-name keys with `null` for closed days, which contradicts both the shipped entity behavior (`{"1":...}` numeric-string keys, missing key = closed) and this spec's stored format. That requirement should be superseded by a `business-profile` delta before or at archive time.

# Spec: data-access

> Reference: `docs/PRD.md` §3.4 (Approach Técnico), §5.1 RF1–RF7; engram obs 453 (testing-capabilities); proposal §Approach
> Change: feat-db-layer
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe exponer una capa de acceso a datos sobre `*sql.DB` que centralice todas las queries SQL por tabla, usando prepared statements exclusivamente, errores semánticos en español, sentinels reutilizables, interfaces pequeñas definidas en el paquete consumidor y una cobertura de tests superior al 80%. Esta capacidad es la meta-capacidad que define cómo se construyen los repositorios de `bookings`, `clients`, `services`, `professionals`, `schedules`, `business_hours_exception` y `pending_alerts`, y cómo se prueban.

## Requirements

### Requirement: Repository pattern with `context.Context` first

Every repository MUST be a struct that holds a `*sql.DB`. Every public method that performs I/O MUST accept a `context.Context` as its first parameter and MUST pass that context to all `database/sql` calls. No method may spawn a goroutine that outlives the context.

#### Scenario: Method signature includes `ctx context.Context`

- GIVEN the source code of any `*Repo` type under `internal/repository/`
- WHEN the public method signatures are reviewed
- THEN every method that performs SQL MUST list `ctx context.Context` as its first parameter

#### Scenario: Context cancellation propagates to the database call

- GIVEN a context that is already cancelled
- WHEN any repository method is called with that context
- THEN the underlying `database/sql` call MUST return a context-cancellation error promptly and the method MUST wrap that error in the standard error-wrapping format

### Requirement: Prepared statements with `?` placeholders only

Every SQL statement executed through a repository MUST use `?` placeholders. No method may concatenate user-supplied values into a SQL string. Table and column names that come from the application code MUST be selected from a fixed allowlist defined in the repository file (not from user input).

#### Scenario: Search query uses parameter binding

- GIVEN a `SearchFTS(ctx, query)` method on the clients repository
- WHEN the implementation is reviewed
- THEN the SQL MUST contain a `?` placeholder for the query parameter, and the query string MUST be passed via a positional argument to the prepared statement

#### Scenario: Table name is not parameterized from user input

- GIVEN the repository implementation
- WHEN the source code is reviewed
- THEN there MUST NOT be any `fmt.Sprintf` or string concatenation that builds a SQL identifier from a value that originated outside the file's own constants

### Requirement: Semantic error wrapping in Spanish

Every error returned by a repository method MUST be wrapped with `fmt.Errorf("...: %w", err)` and the message MUST be a semantic Spanish string. Raw `database/sql` errors MUST NOT be propagated directly to the caller. Stack traces MUST NOT appear in the error message.

#### Scenario: Wrap with semantic message

- GIVEN a `GetByID(ctx, id)` method whose underlying `QueryRow` returns `sql.ErrNoRows`
- WHEN the method is called with an unknown ID
- THEN the returned error MUST be a `repository.ErrNotFound` (see sentinel requirement below) or a wrapped error whose message is a Spanish phrase identifying the missing resource, e.g. `cliente con id 'xyz' no encontrado`

#### Scenario: No stack trace in the message

- GIVEN any error returned by a repository method
- WHEN the error's `Error()` string is inspected
- THEN the string MUST NOT contain substrings such as `goroutine`, `.go:`, or a Go file path

### Requirement: Sentinel errors in `errors.go`

The package `internal/repository` MUST define an `errors.go` file that exports at least the following sentinels:

- `ErrNotFound` — the requested entity does not exist
- `ErrConflict` — a uniqueness or foreign-key constraint was violated
- `ErrInvalidInput` — the input failed application-level validation

All three MUST be usable with `errors.Is` and MUST be returned as wrapped errors (via `fmt.Errorf("...: %w", repository.ErrNotFound)`) by repository methods that need to signal those conditions.

#### Scenario: `errors.Is` resolves the sentinel

- GIVEN a repository method that wraps its error with `ErrNotFound`
- WHEN the caller does `if errors.Is(err, repository.ErrNotFound) { ... }`
- THEN the branch MUST be taken

#### Scenario: Three sentinels exported

- GIVEN the package `internal/repository`
- WHEN the exported names are enumerated
- THEN `ErrNotFound`, `ErrConflict` and `ErrInvalidInput` MUST be among them

### Requirement: Interfaces defined where they are consumed

Consumers of the repositories (e.g., the MCP handlers in `internal/mcp/`) MUST depend on small interfaces (e.g., `BookingsRepository`, `ClientsRepository`), not on the concrete `*Repo` struct. The interfaces MUST live in the consumer package, not in `internal/repository/`. Each interface MUST list only the methods that consumer actually uses.

#### Scenario: Interface lives in the consumer package

- GIVEN the MCP handler for booking-related tools (in `internal/mcp/`)
- WHEN the handler's dependencies are reviewed
- THEN the dependency MUST be typed as an interface (e.g., `BookingsRepository`) declared in the `internal/mcp` package, and the concrete `*repository.BookingsRepo` MUST be assigned to it at wiring time

#### Scenario: Interface is narrow

- GIVEN a `BookingsRepository` interface used by a single handler
- WHEN the interface definition is reviewed
- THEN the interface MUST list only the methods that handler actually calls; methods of the concrete `*Repo` that the handler does not need MUST NOT appear on the interface

### Requirement: Test-first development (TDD)

Every repository method MUST have a `*_test.go` companion that was written first (red-green-refactor). The companion MUST cover at least the happy path, the not-found case, and any input-validation failure cases. Tests MUST run under `go test -v -race ./...` with the race detector enabled.

#### Scenario: Companion test file exists

- GIVEN any `internal/repository/<entity>.go` source file with a public method
- WHEN the same directory is listed
- THEN a file named `<entity>_test.go` MUST exist with at least one test per public method

#### Scenario: Tests pass with the race detector

- GIVEN the test suite for `internal/repository/`
- WHEN `go test -v -race ./internal/repository/...` is executed
- THEN the command MUST exit with status 0 and MUST NOT report any data race

### Requirement: `go-sqlmock` for CRUD, real in-memory SQLite for FTS sync

The default test strategy is `go-sqlmock` (in-memory, no real driver). The single exception is the FTS5 trigger integration test, which MUST run against real in-memory SQLite because `go-sqlmock` cannot simulate trigger side effects. The trigger integration test MUST live in `internal/db/database_test.go` and MUST cover both `clients_fts` and `services_fts` (insert, update, delete).

#### Scenario: CRUD repo test uses go-sqlmock

- GIVEN a CRUD test for `ClientsRepo.GetByID`
- WHEN the test file is reviewed
- THEN the test MUST use `go-sqlmock` to set up expectations and the `sql.DB` returned to the repository MUST be the mock, not a real SQLite handle

#### Scenario: FTS trigger test uses real in-memory SQLite

- GIVEN the FTS5 sync integration test
- WHEN the test file is reviewed
- THEN it MUST use `sql.Open("sqlite", ":memory:")` (or equivalent) and MUST run the schema bootstrap before the assertion, so that the real `AFTER INSERT/UPDATE/DELETE` triggers execute

#### Scenario: FTS trigger test covers all three operations

- GIVEN the FTS5 sync integration test
- WHEN the test body is executed
- THEN it MUST demonstrate that an `INSERT`, an `UPDATE` and a `DELETE` against `clients` and `services` each keep the corresponding `*_fts` table synchronized

### Requirement: Coverage target ≥ 80% in `internal/repository/`

The test suite for `internal/repository/` MUST achieve a line coverage of at least 80% when measured with `go test -cover`. The coverage profile MUST be reviewed as part of the per-PR pre-flight.

#### Scenario: Coverage threshold met

- GIVEN the test suite for `internal/repository/`
- WHEN `go test -v -race -cover ./internal/repository/...` is executed
- THEN the reported line coverage for the package MUST be ≥ 80%

#### Scenario: Coverage reported in PR description

- GIVEN any PR that adds or modifies files under `internal/repository/`
- WHEN the PR description is reviewed
- THEN it MUST include the output of the `go test -cover` run for `internal/repository/` (or a summary that meets the threshold)

### Requirement: Idempotent `GetBusinessProfile` (lazy-init)

`GetBusinessProfile(ctx)` MUST be the only sanctioned way to read the singleton row. It MUST attempt `INSERT OR IGNORE` of a placeholder row before issuing the `SELECT`, so that a fresh install never returns an empty result. The behavior is a property of the data-access layer (the repository method), not of the SQL schema alone.

#### Scenario: First call on a fresh install returns a row

- GIVEN a fresh database with no row in `business_profile`
- WHEN `GetBusinessProfile(ctx)` is called for the first time
- THEN the call MUST return a non-nil `*BusinessProfile` with `ID = 'singleton'` and MUST have inserted a placeholder row

#### Scenario: Second call returns the same row

- GIVEN `GetBusinessProfile(ctx)` has been called once
- WHEN it is called again
- THEN the returned `*BusinessProfile` MUST be the same logical row (same `ID`), and the table MUST still contain exactly one row

### Requirement: No external runtime dependencies (per ADR-0005)

The repository package MUST NOT import any new third-party library beyond what is already in `go.mod` after Fase 1 (specifically `modernc.org/sqlite`, `github.com/DATA-DOG/go-sqlmock` and `github.com/google/uuid`). All other functionality MUST be implemented using the Go standard library.

#### Scenario: No new external imports

- GIVEN the source code under `internal/repository/` and `internal/model/`
- WHEN the import statements are reviewed
- THEN the only third-party imports MUST be from `modernc.org/sqlite`, `github.com/DATA-DOG/go-sqlmock` and `github.com/google/uuid`

### Requirement: Env-var driven config is the pattern (deferred to Fase 2)

The data-access layer MUST be designed so that a future Fase 2 change can introduce env-var driven configuration (per ADR-0007) without changing the repository contracts. Specifically: the `*Repo` constructors MUST accept the `*sql.DB` (already opened) as their dependency, NOT the path to a database file. Fase 2 will introduce `internal/config` (with the `MCP_BIND` / `MCP_PORT` env-var reader and the `.env` parser) and wire the open `*sql.DB` into the repositories; Fase 1 MUST NOT implement that wiring.

#### Scenario: Repo constructor takes `*sql.DB`, not a path

- GIVEN the constructor of any `*Repo` type
- WHEN the signature is reviewed
- THEN it MUST accept `*sql.DB` (or a wrapper that exposes it) and MUST NOT accept a file path or any other I/O configuration

#### Scenario: Env vars are not read in Fase 1

- GIVEN the source code under `internal/repository/`, `internal/model/` and `internal/db/`
- WHEN the source is searched for `os.Getenv`
- THEN there MUST NOT be any call to `os.Getenv` for the keys `MCP_BIND` or `MCP_PORT` (Fase 2 work)

## Notes

- This is the meta-capability for the entire `feat/db-layer` change. The other eight capabilities (`business-profile`, `business-hours-exception`, `professionals`, `schedules`, `services`, `clients`, `bookings`, `pending-alerts`) all consume the conventions defined here.
- The strict TDD requirement is what makes the ≥ 80% coverage target a hard guarantee, not an aspiration. Per the testing capabilities observation (obs 453), no test files exist yet; this is the first feature in the project that creates them.
- The proposal's third PR (complex repos with `CheckAvailability`) is the largest file in the change. The TDD table-driven approach is what makes it tractable: each of the 5 steps of the chain has its own sub-test, and the happy path has an end-to-end sub-test.
- Env-var driven config is mentioned here so the design preserves the seam; implementation is Fase 2 work per the proposal's out-of-scope section and ADR-0007.
- See the proposal's "Approach" section for the chained-PR strategy and per-PR LOC budgets.

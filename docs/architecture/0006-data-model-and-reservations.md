# ADR-0006: Data model and reservation flow design

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

The project needs to persist business operating hours, professional
schedules, and client bookings in a way that supports the
`check_availability()` flow documented in `docs/PRD.md §3.7.13`. Each
of the five design decisions below was made during the 2026-06-25
review and has alternatives that were evaluated and rejected. This ADR
documents the **why** behind the schema choices; the **what** (the
schema itself) is the canonical reference in `docs/PRD.md §3.7`.

The decisions together aim for:

- A model that supports the full reservation flow (param resolution →
  end_datetime → 5-step check_availability → create_booking → alert)
- Cross-platform SQLite with FTS5 full-text search
- No external runtime dependencies (per [ADR-0005](./0005-optional-external-tools.md))
- Minimal complexity (each piece has a clear single responsibility)

## Decision 1: `business_hours` as JSON column in `business_profile`

`business_hours` is stored as a TEXT column (JSON content) inside the
singleton `business_profile` row. The structure is a per-weekday
object with `{ open, close }` for open days and `null` for closed days.
See `docs/PRD.md §3.7.2` for the full schema and query example.

**Rejected alternatives**:

- **Separate `business_hours` table**: would have 7 rows max
  (one per weekday), which is overkill for what is logically
  "config of the business profile". Adds a table and a JOIN
  for every "is the business open on day X?" query.
- **Column-per-weekday in `business_profile`** (e.g.
  `monday_open`, `monday_close`, ...): would bloat the
  `business_profile` table and complicate queries ("what time
  do we close on Wednesdays?").

**Trade-off**: we lose SQL-direct queries on "what time does the
business close on Saturday?" — we have to use `json_extract`
instead. SQLite supports this natively with the JSON1 extension, so
the overhead is negligible. We accept this in exchange for keeping
all the business's static config in one row.

## Decision 2: `business_hours_exception` as a separate table

Date-specific overrides (feriados, eventos, vacaciones) live in a
dedicated `business_hours_exception` table, not in the JSON column.
See `docs/PRD.md §3.7.3` for the full schema.

**Rejected alternatives**:

- **All in the JSON column**: would require re-parsing the JSON
  on every `check_availability` call AND would grow unboundedly
  (each year adds ~10-15 holiday dates plus owner vacations, etc.)
  and lose the `reason` context.
- **National holidays library** (e.g. `github.com/rickar/cal/v2`):
  adds a runtime dependency for what is fundamentally a maintenance
  task the owner does once a year. Violates
  [ADR-0005](./0005-optional-external-tools.md).
- **`national_holidays` table** curated by country: a separate
  catalog table for "every holiday in Argentina since 1900".
  Premature for MVP. The owner only cares about the holidays that
  affect their business; they'll load the ~10 per year manually.

**Trade-off**: the `check_availability` 3a step has TWO queries
now (exception table first, JSON fallback). Acceptable because the
exception table has a UNIQUE index on `exception_date` so the
lookup is O(log n).

**Precedence rule** (documented in `docs/PRD.md §3.7.13 Paso 3a`):
if there's an exception for the requested date, use it. Otherwise
fall back to the JSON weekly schedule.

## Decision 3: `bookings.end_datetime` is denormalized

`bookings.end_datetime` is stored explicitly (computed as
`start_datetime + service.duration_minutes` at insert time) rather
than computed on read.

**Rejected alternatives**:

- **Compute on the fly** with a JOIN to `services`: every
  `check_availability` overlap-check query (the most frequent
  write-path query in the system) would need to JOIN
  `bookings → services` to get the duration. The optimizer can
  help, but it's a hot path.
- **SQLite generated column** (`GENERATED ALWAYS AS ... STORED`):
  SQLite added support in 3.31.0 (2020) but it has limitations on
  expressions involving other tables (can't reference `services`).
  Would need a stored duration in `bookings` anyway.
- **Triggers that auto-update `end_datetime`** when
  `service.duration_minutes` changes: complicates the schema
  and adds runtime cost for an update that happens at most a few
  times per year.

**Trade-off**: if the duration of a service changes after a
booking is made, the booking's `end_datetime` stays at the
original value (consistency over freshness). For an MVP, this is
the right call: service durations are essentially immutable in
practice, and the historical accuracy is more important than the
sync cost.

## Decision 4: FTS5 sync via SQL triggers, not Go code

The FTS5 virtual tables (`clients_fts`, `services_fts`) use
`content='source'` to mirror their source tables, and the
synchronization is done by SQL triggers (`AFTER INSERT`,
`AFTER UPDATE`, `AFTER DELETE`) on the source tables — NOT by
Go code in the repository layer.

**Rejected alternatives**:

- **Go code in the repository layer** that explicitly
  inserts/updates/deletes the FTS rows: every write to
  `clients` or `services` would need to know to also write
  to the FTS table. A repository with this responsibility
  would have to be VERY careful not to forget, and bulk
  imports / migrations would silently desync.
- **No sync at all** (just create the FTS tables):
  FTS5 with `content='source'` is useless without sync. The
  FTS would always return zero results. This was the actual
  state of the project in the foundation phase (gap
  documented in engram obs 464).

**Trade-off**: SQL triggers are slightly opaque to Go developers
who don't know SQL well. Mitigated by:

- The trigger SQL is documented verbatim in
  `docs/PRD.md §3.7.10` (copy-paste, don't reinvent)
- The triggers are unit-tested as part of the `feat/db-layer`
  phase
- The Go code is the same regardless of whether the FTS exists

## Decision 5: Reservation flow as a 5-step validation chain

The `check_availability` tool is implemented as a sequence of
validations (3a through 3e) that return a single semantic error
message on the first failure. See `docs/PRD.md §3.7.13` for the
complete flow.

The 5 validations are:

- **3a**: ¿El negocio está abierto ese día? (exception first, then JSON)
- **3b**: ¿El Profesional trabaja ese día? (`schedules`)
- **3c**: ¿El slot cabe en el horario? (end ≤ close)
- **3d**: ¿Overlap con otra reserva? (`bookings` query)
- **3e**: ¿Slot no en el pasado? (datetime comparison)

**Rejected alternatives**:

- **Single big query** that joins all tables and returns a
  generic "conflict" or "no conflict" boolean: would lose the
  ability to return a specific error message ("profesional no
  trabaja los domingos" vs "ya tiene reserva a las 15:00"). The
  LLM needs the specific reason to communicate it to the user.
- **State machine on the booking**: a more formal
  "available / unavailable / pending" status. Overkill for MVP;
  the validation chain handles all the cases we need.

**Trade-off**: the validation chain has 5 sequential queries in
the worst case. We considered parallelizing 3b/3c/3d (independent
checks), but decided the latency gain isn't worth the code
complexity for MVP. Single-threaded sequential is ~5 ms on a
modest database; well within the p95 < 100 ms target from
`docs/PRD.md §5.2 RNF`.

## Consequences

**Positive** (all 5 decisions together):

- The reservation flow is fully testable: each of the 5
  validations can be unit-tested independently
- No external runtime dependencies (per [ADR-0005](./0005-optional-external-tools.md))
- The schema supports the `find next available slot` use case
  (`slot_interval_minutes` + `business_hours` JSON + `schedules`
  table together provide everything)
- Historical consistency: `end_datetime` is preserved even if
  service durations change later
- FTS5 works correctly (sync triggers prevent the silent
  zero-results bug that was a foundation-phase gap)

**Negative** (all 5 decisions together):

- `check_availability` requires up to 5 sequential queries
  (acceptable per the p95 < 100 ms target)
- The hybrid JSON + table model is more complex than "all
  SQL" or "all JSON" — the reader needs to know both
  patterns
- Some denormalization (`end_datetime`): small risk of drift
  if a `service.duration_minutes` change is not accompanied by
  a `bookings.end_datetime` recompute

**Rejected alternatives (global)**:

- **NoSQL / document store** (e.g. Firestore, MongoDB):
  contradicts the project's "self-hosted, lightweight"
  positioning. Adds a runtime dependency.
- **Graph DB** (e.g. Neo4j, Cayley): overkill for the data
  shape. Bookings are naturally relational.
- **Postgres / MySQL**: heavier than SQLite, requires
  server-side setup. Contradicts [ADR-0001](./0001-no-docker.md)
  (no Docker / no external services) and the project
  philosophy.

## References

- `docs/PRD.md §3.7` — canonical schema reference (the WHAT)
- `docs/PRD.md §3.7.13` — reservation flow detail
- [ADR-0001](./0001-no-docker.md) — no Docker philosophy
- [ADR-0002](./0002-user-level-install.md) — user-level install
- [ADR-0004](./0004-naming-conventions.md) — `bookings` not `appointments`
- [ADR-0005](./0005-optional-external-tools.md) — no external deps
- Commit `99eca18` — when the PRD §3.7.3 + renumbering + flow updates were added
- engram obs 464 — project state with the 12 known db-layer gaps

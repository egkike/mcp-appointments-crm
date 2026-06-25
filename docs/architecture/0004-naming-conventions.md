# ADR-0004: Project naming conventions

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

The initial SQLite schema in `internal/db/database.go` and the original PRD /
SDD used slightly different names for the same concepts. This created
cognitive overhead (e.g., is the reservations table called `appointments` or
`bookings`? Is the duration field `duration_mins` or `duration_minutes`?).
Drift between code and docs makes it harder to reason about the system.

The conventions must be settled before the db-layer (Fase 1) extends the
schema, otherwise the new code will inherit the inconsistencies.

## Decision

The canonical names are:

| Concept | Canonical name | Rejected alternative |
|---|---|---|
| Reservations table | `bookings` | `appointments` |
| Reservation duration | `duration_minutes` | `duration_mins` |
| Reservation start | `start_datetime` | `start_time` |
| Reservation end | `end_datetime` | `end_time` |
| Messenger channel fields | `messenger_platform`, `messenger_id` in **`business_profile`** | In `clients` |
| Repository (Go): collections | Plural (`BookingsRepo`, `ClientsRepo`) | Singular |
| Model (Go): aggregates | Singular (`Booking`, `Client`) | Plural |

These names apply to:
- SQL table names and column names
- Go struct field names (with `db:` tags where names differ)
- Go type names and package conventions
- Variable names in handlers and services
- The MCP tool argument names exposed to Hermes

The conventions are documented in `docs/PRD.md` §3.5 (the "Convenciones de
nomenclatura" blockquote) and in the package comment of
`internal/model/doc.go`.

## Consequences

**Positive**:
- Code and docs speak the same language
- New contributors and future sessions have a single source of truth
- Migrations from the old names to the new names are documented as part of
  Fase 1 (db-layer)
- Go conventions (plural repos, singular models) match the standard
  pattern in Go projects

**Negative**:
- Existing code in `internal/db/database.go` uses the old names; renaming
  is part of Fase 1 (db-layer) and adds to that phase's work
- Any external clients (none yet) that know the old table names will break

**Rejected alternatives**:
- **Keep both names with aliases**: doubles the cognitive load, no benefit
- **Rename later (post-MVP)**: accumulates technical debt that compounds
  with every line of new code

## References

- `docs/PRD.md` §3.5 — Convenciones de nomenclatura blockquote
- `internal/model/doc.go` — package comment
- `internal/db/database.go` — current schema (to be updated in Fase 1)
- Commit `ca7d0d9` — initial alignment (partial: bookings + duration_minutes + 0750)
- Commit `194888d` — broader alignment as part of the no-Docker work

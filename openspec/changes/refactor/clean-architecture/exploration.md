# Clean Architecture Gaps Analysis — mcp-appointments-crm

**Change**: refactor/clean-architecture
**Date**: 2026-07-29
**Status**: Complete

---

## Current Architecture Summary

```
internal/                               Layer          Depends on
├── model/               (8 files)      [DATA]         google/uuid
│   ├── booking.go                     anemic struct   (only BookingStatus has behavior)
│   ├── client.go                      anemic struct
│   ├── service.go                     anemic struct
│   ├── professional.go                anemic struct
│   ├── schedule.go                    anemic struct
│   ├── business_profile.go            anemic struct
│   ├── business_hours_exception.go    anemic struct
│   ├── pending_alert.go              anemic struct
│   ├── account.go                    anemic struct
│   └── uuid.go                       utility
├── repository/          (15 files)    [DATA ACCESS]   model, auth, modernc/sqlite
│   ├── bookings.go           — CRUD + 5-step CheckAvailability (220 LOC of domain logic!)
│   ├── clients.go            — CRUD + FTS5 + GetOrCreate (business decision in repo)
│   ├── services.go           — CRUD + FTS5 + validateService
│   ├── professionals.go      — CRUD + auth gate + validateProfessional
│   ├── schedules.go          — CRUD + auth gate + validateDayOfWeek/ScheduleTimes
│   ├── accounts.go           — CRUD + audit logging + single-owner invariant check
│   ├── business_profile.go   — lazy-init singleton + validation
│   ├── business_hours_exception.go — CRUD + validation
│   ├── pending_alerts.go     — CRUD + Fase 1 type allowlist + auth gate
│   ├── datetime.go           — parse/format utilities
│   ├── validation.go         — shared regex/validation helpers
│   ├── errors.go             — SemanticError + sentinels + all ErrCode constants
│   └── auth_helpers.go       — requireCaller/requireRole/requireClientMatch
│
├── auth/                (5 files)     [AUTH]         database/sql (resolver), net/http (middleware)
│   ├── caller.go         — Caller struct + context propagation
│   ├── resolver.go       — CallerResolver (queries accounts/clients)
│   ├── middleware.go     — HTTP middleware (X-Caller-Id → RBAC → audit)
│   ├── doc.go
│   └── _test.go
│
├── db/                  (3 files)     [DB BOOTSTRAP] modernc/sqlite
│   ├── database.go       — NewDatabase, DSN builder, pragma verification
│   ├── schema.go         — All DDL (8 tables + FTS5 + triggers + indexes)
│   └── database_test.go
│
├── config/              (1 file)      [CONFIG]       doc only — no implementation
│   └── doc.go
│
└── validation/          (1 file)      [VALIDATION]   doc only — no implementation
    └── doc.go
```

**Dependency graph (current)**:
```
repository ──→ model ──→ google/uuid
    │
    ├──→ auth (caller context, role constants)
    └──→ modernc/sqlite (directly in errors.go)

auth ──→ database/sql (CallerResolver uses *sql.DB directly)
     └──→ net/http (middleware)

db ──→ modernc/sqlite

model ──→ google/uuid
```

**Critically missing**: No `internal/domain/`, no `internal/application/`, no `cmd/`, no `internal/mcp/`. The MCP server transport layer does not exist yet.

---

## What's Already Right ✓

### 1. Model has domain behavior on value types
`internal/model/booking.go:13-33` — `BookingStatus` has `ValidTransitions()` and `IsValidTransition()`. This is *real* domain behavior on a value type, exactly where it belongs.

### 2. Clean error abstraction
`internal/repository/errors.go` — `SemanticError` with `ErrCode` is a proper domain-level error type. The `Unwrap()` method enables `errors.As`/`errors.Is` chains. Sentinels (`ErrNotFound`, `ErrConflict`, `ErrInvalidInput`) provide clean control flow.

### 3. Auth is a cleanly separated concern
`internal/auth/` — `Caller`, `CallerResolver`, `AuthMiddleware` form a self-contained auth layer with no dependency on `model` or `repository`. Context propagation via `WithCaller`/`FromContext` follows Go idioms.

### 4. Schema is entirely separated from data access
`internal/db/schema.go` — All DDL is centralized. No SQL schema leak into repository files. Pragma verification in `database.go` is robust.

### 5. Prepared statements everywhere
Every query uses `?` placeholders. No raw SQL concatenation found in any file. ✓

### 6. Context propagation through all layers
Every repository method accepts `context.Context`. ✓

### 7. Test infrastructure is well-structured
`internal/repository/testutil_test.go` — `newMockDB()` with `t.Cleanup` for expectation verification, role-specific context helpers (`adminCtx`, `ownerCtx`, `staffCtx`, `clientCtx`).

### 8. Dependency graph has some clean separation
- `auth` does not depend on `repository` (resolver receives `*sql.DB`)
- `model` depends only on `google/uuid` (a leaf package)
- `db` depends only on `modernc/sqlite`

---

## Concrete Gaps with File References

### GAP-1: No repository interfaces — domain depends on SQL (HIGH)

**Files**: `internal/repository/bookings.go:20-22`, `clients.go:15`, `services.go:14`, `professionals.go:18`, `schedules.go:15`, `accounts.go:18`, `business_profile.go:17`, `business_hours_exception.go:16`, `pending_alerts.go:15`

**Problem**: Every repo is a concrete struct with `*sql.DB` directly embedded. There are zero repository interfaces. In Clean Architecture, the **domain layer defines repository interfaces**, and concrete implementations live in the infrastructure layer. Currently:
- You cannot swap a repo implementation (e.g., for testing, for a different DB)
- Domain entities have no way to call persistence abstractly
- All business logic that needs data access is forced into the repo

**Evidence**:
```go
// internal/repository/bookings.go:20-22
type BookingsRepo struct {
    db *sql.DB
}
```
There's no `domain.BookingRepository` interface anywhere.

### GAP-2: Business logic lives in repositories (HIGH)

**Files**:
- `internal/repository/bookings.go:459-679` — `CheckAvailability` (220 lines of pure domain logic)
- `internal/repository/bookings.go:62-146` — `CreateBooking` (orchestration + auth + duration query + overlap check)
- `internal/repository/bookings.go:295-434` — `RescheduleBooking` (orchestration + auth + FSM + duration + overlap)
- `internal/repository/clients.go:83-108` — `GetOrCreate` (business decision: "create if not exists")
- `internal/repository/professionals.go:28-36` — `validateProfessional` (domain invariants)
- `internal/repository/services.go:25-36` — `validateService` (domain invariants: price, duration)
- `internal/repository/schedules.go:25-45` — `validateScheduleTimes` (domain invariants)
- `internal/repository/accounts.go:71-117` — `Create` includes single-owner pre-check (domain rule)

**The `CheckAvailability` method is the worst offender**: 220 lines containing the full 5-step validation chain (business hours, professional schedule, slot within hours, overlap, past check). This is pure domain logic with zero persistence abstraction — it queries 5 different tables directly.

### GAP-3: No domain or application layers (HIGH)

**Problem**: The Clean Architecture onion has 4 layers:
```
Domain → Application → Infrastructure (repo/db) → Transport (MCP/HTTP)
```

Currently only exists:
```
[model + repo (mixed)] → [auth] → [db]
```

Missing entirely:
- `internal/domain/` — entities with behavior, repository interfaces, domain services, domain errors
- `internal/application/` — use cases / application services, DTOs, input ports, output ports

### GAP-4: Input/DTO types defined inside repositories (MEDIUM)

**File**: `internal/repository/bookings.go:30-47`, `436-446`

```go
// Input types in repository package
type CreateBookingInput struct { ... }
type CreateBookingResult struct { Booking *model.Booking }
type CheckAvailabilityParams struct { ... }
type CheckAvailabilityResult struct { Available bool }
```

These are application-layer contracts — they describe *what* the system should do, not *how* to persist it. Having them in the repository means:
- You must import the persistence layer to invoke a business operation
- The use-case boundary is invisible

### GAP-5: Domain validation mixed with persistence (MEDIUM)

**Files**:
- `internal/repository/validation.go` — regex/format validation (could be shared)
- `internal/repository/business_profile.go:29-57` — `validateBusinessProfile`
- `internal/repository/services.go:25-36` — `validateService`
- `internal/repository/professionals.go:28-36` — `validateProfessional`
- `internal/repository/accounts.go:35-46` — `validateAccount`
- `internal/repository/schedules.go:25-45` — `validateScheduleTimes`
- `internal/repository/business_hours_exception.go:32-64` — inlined validation in Create
- `internal/repository/pending_alerts.go:30-36` — `validateAlertType`

Some of these are format validators (fine at boundary), others are **domain invariants** that should live in the domain. `validateService` checking `Price <= 0` is a domain invariant — a Service *cannot exist* with a non-positive price. That belongs in a `Service` constructor method, not in the repo.

### GAP-6: Domain error types defined in repository package (MEDIUM)

**File**: `internal/repository/errors.go`

`SemanticError`, `ErrNotFound`, `ErrConflict`, `ErrInvalidInput`, `ErrUnauthenticated`, and all `ErrCode*` constants are defined in the repository package. In Clean Architecture:
- **Domain errors** belong in `internal/domain/errors.go`
- The repository should *translate* infrastructure errors to domain errors, not *define* them
- Currently, even if you extracted a use case to `internal/application/`, it would need to import `internal/repository` just to use `SemanticError`

### GAP-7: Auth helpers coupled to repository package (MEDIUM)

**File**: `internal/repository/auth_helpers.go`

`requireCaller`, `requireRole`, `requireClientMatch` are in the repo package. Authorization is a cross-cutting concern. Having it in the repo means:
- Use cases cannot auth-gate without importing the repo package
- The repo acts as both persistence + auth gate, violating SRP
- This logic should be in `internal/auth/` (which already exists!) or `internal/application/auth/`

### GAP-8: No MCP server / transport layer (HIGH — but opportunity)

**Problem**: The entire transport layer is absent — no `cmd/`, no `internal/mcp/`. The MCP server that will expose these repos to the LLM (Hermes) hasn't been built yet.

**Opportunity**: This is the BEST time to introduce Clean Architecture layers. If the MCP handlers are written to call repos directly, the architecture will be locked into the current coupling. By introducing domain + application layers first, the handlers will inject use cases and never touch repos.

### GAP-9: `model.NewUUID()` called from repositories (LOW)

**Files**: `internal/repository/bookings.go:97`, `internal/repository/clients.go:92`, `internal/repository/professionals.go:79`

ID generation is an infrastructure concern called inside repos. If you wanted to move it, the domain could define an `IDGenerator` interface. Minor issue.

### GAP-10: All tests are SQL-coupled (MEDIUM)

**Files**: All `*_test.go` in `internal/repository/`

**Problem**: Every test uses sqlmock. Business logic like `CheckAvailability` can only be tested through SQL mocking, making tests:
- Brittle (break on query changes)
- White-box (know internal query shapes)
- Slow (even mocked, SQL execution has overhead)
- Unable to test edge cases without SQL row mocking

After Clean Architecture migration, domain logic would be testable with pure Go unit tests (no mocks), while repository tests would only test persistence mapping.

---

## Recommended Target Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    TRANSPORT LAYER                       │
│  internal/mcp/   (handlers, server, DTOs)               │
│  cmd/            (main entry point)                     │
│     ┌─────────── MCP handlers call use cases ─────────┐ │
│     ▼                                                   │ │
│  ┌─────────────────────────────────────────────────┐    │ │
│  │            APPLICATION LAYER                      │    │ │
│  │  internal/application/                            │    │ │
│  │    ├── booking/create_booking.go  (use case)      │    │ │
│  │    ├── booking/check_availability.go (use case)   │    │ │
│  │    ├── booking/cancel_booking.go  (use case)      │    │ │
│  │    ├── client/register_client.go  (use case)      │    │ │
│  │    └── ...  (one file per use case)               │    │ │
│  └──────────── use cases depend on domain interfaces ─┘  │ │
│     │                                                     │ │
│     ▼                                                     │ │
│  ┌─────────────────────────────────────────────────┐    │ │
│  │            DOMAIN LAYER ★ NEW                     │    │ │
│  │  internal/domain/                                 │    │ │
│  │    ├── booking.go        (entity + behavior)      │    │ │
│  │    ├── client.go         (entity)                 │    │ │
│  │    ├── service.go        (entity + invariants)    │    │ │
│  │    ├── repositories.go   (interfaces only!)       │    │ │
│  │    ├── errors.go         (SemanticError, sentinels)│    │ │
│  │    └── services.go       (domain service ifaces)  │    │ │
│  └──── domain knows nothing about infra or transport ─┘  │ │
│     │                                                     │ │
│     ▼                                                     │ │
│  ┌─────────────────────────────────────────────────┐    │ │
│  │          INFRASTRUCTURE LAYER                     │    │ │
│  │  internal/repository/sqlite/   ★ REFACTORED       │    │ │
│  │    ├── booking_repo.go   (implements domain iface)│    │ │
│  │    ├── client_repo.go    (implements domain iface)│    │ │
│  │    └── ...                                        │    │ │
│  │  internal/db/             (KEEP — DDL + connection)│    │ │
│  │  internal/auth/           (KEEP — already clean)   │    │ │
│  │  internal/validation/     (KEEP — leaf package)    │    │ │
│  └───────────────────────────────────────────────────┘    │
└────────────────────────────────────────────────────────────┘
```

### Key architectural rules

| Rule | Enforced by |
|------|-------------|
| Domain depends on NOTHING (except stdlib, maybe google/uuid) | Package imports |
| Application depends on domain (interfaces, entities) | Package imports |
| Repository/sqlite implements domain interfaces | Compiler (implicit interface satisfaction) |
| MCP handlers inject use cases, never call repos directly | Constructor injection |
| Repository interfaces defined in domain, NOT in infra | File location |
| Domain errors defined in domain package | File location |
| DTOs defined in application layer | File location |

---

## Suggested Migration Strategy

### Phase 1 — Extract domain layer (HIGH priority, 3-5 sessions)
**Effort**: High
**Risk**: Low (additive — no existing code breaks, new package coexists)

| Step | Files affected | What to do |
|------|---------------|------------|
| 1a | `internal/model/` → `internal/domain/` | Move all model structs. Add behavior methods (constructors with validation on `Service`, `Client`, `Professional`). Keep JSON tags on entities (they're domain, not transport). |
| 1b | `internal/repository/errors.go` → `internal/domain/errors.go` | Move `SemanticError`, sentinels, all `ErrCode*` constants. Remove `isUniqueViolation` (infra-specific). |
| 1c | `internal/domain/repositories.go` | Define interfaces: `BookingRepository`, `ClientRepository`, `ServiceRepository`, `ProfessionalRepository`, `ScheduleRepository`, `BusinessProfileRepository`, `BusinessHoursExceptionRepository`, `PendingAlertRepository`, `AccountRepository`. Each with methods matching what the use cases need. |
| 1d | `internal/repository/` | Existing repos continue to compile. They will later implement the domain interfaces. |

### Phase 2 — Extract application layer (HIGH priority, 3-5 sessions)
**Effort**: High
**Risk**: Medium (existing repos won't know about use cases)

| Step | What to do |
|------|------------|
| 2a | Create `internal/application/booking/check_availability.go` — extract the 5-step logic from `bookings.go:459-679`. The use case receives domain interfaces via constructor, calls them, returns domain errors. |
| 2b | Create `internal/application/booking/create_booking.go` — extract orchestration from `bookings.go:62-146`. Handles auth, duration query, overlap check as a unit. |
| 2c | Create `internal/application/booking/cancel_booking.go` — FSM transition as a use case. |
| 2d | Create `internal/application/booking/reschedule_booking.go` — reschedule as a use case. |
| 2e | Create `internal/application/client/` — `register_client.go`, `find_client.go`, etc. |
| 2f | Move input types (`CreateBookingInput`, `CheckAvailabilityParams`, etc.) into the application layer. |

### Phase 3 — Refactor existing repos to implement interfaces (MEDIUM priority, 2-3 sessions)
**Effort**: Medium
**Risk**: Low (mechanical change)

| Step | What to do |
|------|------------|
| 3a | Move `internal/repository/*.go` to `internal/repository/sqlite/*.go` |
| 3b | Make each repo implement the corresponding domain interface |
| 3c | Add constructor injection of `*sql.DB` (already done) |
| 3d | Move SQLite-specific error mapping into the repo (translate `sqlite.Error` → domain `SemanticError`) |
| 3e | Update tests to work with the new package structure |

### Phase 4 — Fix auth helpers (LOW priority, 1 session)
**Effort**: Low

| Step | What to do |
|------|------------|
| 4a | Move `requireCaller`, `requireRole`, `requireClientMatch` into `internal/auth/` or create `internal/application/auth/` |
| 4b | Use cases call auth helpers directly, not through repos |

### Phase 5 — Create MCP transport layer (WHEN BUILDING SERVER, 2-3 sessions)
**Effort**: Medium

| Step | What to do |
|------|------------|
| 5a | Create `cmd/mcp-appointments-crm/main.go` |
| 5b | Create `internal/mcp/handlers/` — thin handlers that wire use cases |
| 5c | Handlers receive use cases via constructor injection |
| 5d | Handlers never import `internal/repository/sqlite` directly |

---

## Effort Summary

| Gap | Effort | Phase | Risk | Value |
|-----|--------|-------|------|-------|
| GAP-1: No repository interfaces | High | P1 | Low | Highest — enables all Clean Architecture |
| GAP-2: Business logic in repos | High | P1/P2 | Medium | Highest — decouples domain from SQL |
| GAP-3: No domain/application layers | High | P1/P2 | Low | Highest — architectural foundation |
| GAP-4: Input types in repos | Med | P2 | Low | High — clarifies boundaries |
| GAP-5: Validation mixed in repos | Med | P1/P4 | Low | Medium — some validation is fine in repos |
| GAP-6: Error types in repo pkg | Med | P1 | Low | High — enables domain independence |
| GAP-7: Auth helpers in repo pkg | Med | P4 | Low | Medium — cross-cutting concern |
| GAP-8: No MCP transport layer | High | P5 | — | Opportunity — do it right from the start |
| GAP-9: ID generation in repos | Low | P4 | Low | Low — cosmetic |
| GAP-10: SQL-coupled tests | Med | P2/P3 | Low | Medium — enables pure domain tests |

---

## Risks

1. **Scope creep**: The migration is substantial. If the team has feature pressure, Phase 1-2 could drag. **Mitigation**: The changes are additive — old repos keep working while new layers are built around them.

2. **Over-engineering**: Go doesn't need full-on Java-style Clean Architecture. **Mitigation**: The proposed target is pragmatic Go — interfaces in the domain package, use cases as single-file structs with constructor injection, not a complex framework.

3. **Testing debt during transition**: While both old and new structures coexist, test coverage could fragment. **Mitigation**: Write new domain tests in pure Go (no mocks), keep existing sqlmock repo tests until P3 is complete.

4. **Auth integration**: Auth helpers are spread across `internal/auth/` and `internal/repository/auth_helpers.go`. Use cases will need auth too. **Mitigation**: Phase 4 consolidates auth into a single package that both repos and use cases can import.

---

## Ready for Proposal

**Yes**. This exploration has identified every gap with concrete file references, proposed a target architecture, and defined an incremental migration strategy. The next step is to create an SDD proposal (`sdd-propose`) that formalizes the scope, approach, and rollback plan for this refactor.

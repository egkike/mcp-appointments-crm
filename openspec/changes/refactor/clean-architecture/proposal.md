# Proposal: Clean Architecture Refactor

> **Change**: refactor/clean-architecture
> **Status**: Proposed
> **Date**: 2026-07-29

## Intent

Migrate the current mixed-layer architecture to a pragmatic Clean/Hexagonal Architecture with clear dependency inversion, without over-engineering or Java-style ceremony. The goal is maintainable, testable code where domain logic is independent of infrastructure concerns.

## Current State

```
internal/
├── model/              # Anemic structs, no behavior
├── repository/         # SQL repos + business logic + auth helpers + errors + validation
├── auth/               # Clean, well-separated
├── db/                 # SQLite bootstrap, fine as-is
├── validation/         # Shared validators, fine as-is
└── config/             # Empty (future)
# cmd/ and internal/mcp/ don't exist yet — will be created in Phase 4+
```

## Identified Gaps

### Gap 1: No domain layer (CRITICAL)
- `internal/model/` has 9 pure data structs (`Booking`, `Client`, `Professional`, `Service`, `BusinessProfile`, `BusinessHoursException`, `PendingAlert`, `Schedule`, `Account`).
- Anemic model: domain behavior exists but lives in `BookingStatus.ValidTransitions()` on the status type, and validators spread across repository methods. Entity structs have no business methods.
- No repository interfaces defined anywhere — repos are concrete structs with `*sql.DB`.

**Files**: `internal/model/*.go` (9 files)

### Gap 2: Business logic in repository layer (CRITICAL)
- `CheckAvailability` (220 LOC, 5-step chain) is a method on `BookingsRepo`.
- Business rules (business hours, professional schedules, overlap detection) live in infra code.
- Cannot test availability logic without a database or sqlmock.

**Files**: `internal/repository/bookings.go:459-679`

### Gap 3: Domain errors in infrastructure layer (HIGH)
- `SemanticError`, `ErrCode*`, and sentinel errors (`ErrNotFound`, `ErrConflict`, `ErrInvalidInput`) live in `internal/repository/errors.go`.
- `ErrUnauthenticated` has TWO conflicting definitions: `repository/auth_helpers.go:14` ("caller not authenticated") and `auth/resolver.go:12` ("unauthenticated") — must consolidate to one during migration.
- Domain-layer concepts (business closed, slot in past, overlap) depend on the repository package.

**Files**: `internal/repository/errors.go`, `internal/repository/auth_helpers.go`, `internal/auth/resolver.go`

### Gap 4: Auth helpers in repository layer (HIGH)
- `requireCaller(ctx)` and `requireClientMatch()` live in `internal/repository/auth_helpers.go`.
- Every package that needs auth must import `internal/repository` just for these helpers.

**Files**: `internal/repository/auth_helpers.go`

### Gap 5: No repository interfaces (HIGH)
- All consumers depend on concrete repo structs.
- Impossible to mock a repo for use-case testing without sqlmock.
- Violates DIP: high-level modules depend on low-level modules.

**Files**: All `internal/repository/*.go`

### Gap 6: No use-case / application layer (MEDIUM)
- No orchestration layer between transport and domain.
- When MCP handlers are built, they would call repos directly — business orchestration leaks into transport.

**Files**: Missing `internal/application/usecase/`

### Gap 7: No DTO separation (MEDIUM)
- Model structs are used directly. No request/response types.
- JSON tags, validation tags, and DB column mapping all in the same struct.

**Files**: `internal/model/*.go`

### Gap 8: No transaction boundary (MEDIUM)
- Each repo operation is its own SQL transaction.
- No `UnitOfWork` or multi-repo atomic operations possible.

**Files**: `internal/repository/*.go`

### Gap 9: Transport layer not yet created (LOW)
- `cmd/` directory doesn't exist yet. `internal/mcp/` doesn't exist yet. Not blocking now, but the architecture must support MCP handlers when they arrive in Phase 5.
- Phase 4 will create `cmd/mcp-server/main.go` with DI wiring for the existing binary.

**Files**: `cmd/` (to be created in Phase 4)

### Gap 10: Testing coupling (MEDIUM)
- Domain/application tests require sqlmock because there's no repo interface.
- 146 tests in `internal/repository/` are sqlmock-coupled — good for infra tests, but business logic tests shouldn't need them.

**Files**: `internal/repository/*_test.go`

## Target Architecture

```
internal/
├── domain/                         # Zero dependencies on infra packages
│   ├── entity/                     # Business entities with behavior
│   │   ├── booking.go
│   │   ├── client.go
│   │   ├── professional.go
│   │   ├── service.go
│   │   ├── business_profile.go
│   │   ├── business_hours_exception.go
│   │   ├── pending_alert.go
│   │   ├── schedule.go
│   │   └── account.go
│   ├── repository/                 # Interfaces (not implementations)
│   │   ├── bookings.go
│   │   ├── clients.go
│   │   ├── professionals.go
│   │   ├── services.go
│   │   ├── business_profile.go
│   │   ├── business_hours_exception.go
│   │   ├── pending_alerts.go
│   │   ├── schedules.go
│   │   └── accounts.go
│   ├── service/                    # Domain services (business logic)
│   │   └── availability.go
│   └── errors.go                   # SemanticError + sentinel errors + error codes
├── application/                    # Depends on domain interfaces, not infra
│   ├── usecase/
│   │   ├── create_booking.go
│   │   ├── cancel_booking.go
│   │   ├── reschedule_booking.go
│   │   ├── check_availability.go
│   │   └── get_booking.go
│   └── dto/
│       ├── create_booking.go
│       ├── cancel_booking.go
│       └── ...
├── repository/                     # SQL implementations → implements domain interfaces
│   ├── bookings.go                 # Now implements domain.BookingRepository
│   ├── errors.go                   # Removed — domain errors moved to domain/
│   ├── auth_helpers.go             # Removed — moved to auth/
│   └── ...
├── auth/                           # ✅ Keep, add requireCaller/requireClientMatch
├── db/                             # ✅ Keep
├── validation/                     # ✅ Keep
├── mcp/                            # Future: wire use cases
└── config/                         # Future
```

## Migration Strategy: Incremental (5 Phases)

### Phase 1 — Extract Domain Layer (additive, zero risk)
- Create `internal/domain/` with entities, repository interfaces, and errors
- Move models → domain entities with behavior
- No existing code changes
- **Effort**: Medium
- **Risk**: None (additive)

### Phase 2 — Application Layer (additive)
- Create `internal/application/` with use cases and DTOs
- Wire use cases to domain interfaces
- No existing code changes
- **Effort**: Medium
- **Risk**: None (additive)

### Phase 3 — Refactor Repositories (modifies existing code)
- Implement domain repository interfaces in `internal/repository/`
- Move business logic → domain services
- Move auth helpers → `internal/auth/`
- Move errors → `internal/domain/errors.go`
- **Effort**: High
- **Risk**: Medium (changes existing code)

### Phase 4 — Fix Dependency Injection
- Wire everything through interfaces at the `cmd/` level
- Ensure infra never imports domain entities
- **Effort**: Low
- **Risk**: Low (after P3 boundaries are clear)

### Phase 5 — Transport Layer (future)
- Build MCP handlers that consume use cases
- Add API DTOs if needed
- **Effort**: Future
- **Risk**: Future (when MCP server is built)

## Non-Goals
- No framework introduction (no DI containers, no repositories-of-repositories)
- No Java-style over-abstraction (interfaces defined where they're consumed, not preemptively)
- No big-bang rewrite
- No database changes
- No API contract changes

## Open Questions
1. Should `SemanticError` remain for domain errors or replace with Go 1.26 standard error patterns?
2. Should `internal/model/` be deleted or kept as deprecated aliases during migration?
3. Transaction boundary: explicit `UnitOfWork` interface or implicit (each use case opens/closes)?

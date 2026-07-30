# Design: Clean Architecture Refactor

> **Change**: refactor/clean-architecture
> **Status**: Designed
> **Date**: 2026-07-29

## Layer Architecture

```
┌─────────────────────────────────────────────────┐
│                  cmd/ (wiring)                   │
│  Constructs concrete implementations, injects    │
│  domains interfaces into application use cases   │
├─────────────────────────────────────────────────┤
│            internal/application/                 │
│  Orchestrates business flows via domain          │
│  interfaces. Depends ONLY on domain/.            │
│  Knows nothing about SQL, HTTP, or transport.    │
├─────────────────────────────────────────────────┤
│              internal/domain/                    │
│  Entities, repository interfaces, domain         │
│  services, domain errors.                        │
│  Zero dependencies on any other package.         │
├─────────────────────────────────────────────────┤
│  internal/repository/   │   internal/auth/       │
│  SQL implementations    │   Auth primitives      │
│  Implements domain/     │   + requireCaller      │
│  repository interfaces  │                        │
├─────────────────────────┴────────────────────────┤
│  internal/db/ │ internal/validation/ │ other     │
│  Infra support packages, no domain imports       │
└─────────────────────────────────────────────────┘
```

**Dependency rule**: Outer layers may depend on inner layers. Inner layers NEVER depend on outer layers.

## Decisiones

### D1 — Package naming: singular vs plural
- **Decision**: `internal/domain/entity/` (singular files: `booking.go`), `internal/domain/repository/` (plural files: `bookings.go`)
- **Rationale**: Entity files describe one thing. Repository interfaces describe a collection responsibility.
- **Consequence**: Consistent with current `internal/model/` pattern (singular structs) and `internal/repository/` (plural files).

### D2 — Repository interfaces: by consumer need, not data table
- **Decision**: Define interfaces in `internal/domain/repository/` with methods the application layer needs, not 1:1 with SQL operations.
- **Rationale**: `FindByID` + `Save` covers most use cases. A dedicated interface per aggregate root prevents bloated interfaces.
- **Consequence**: Some repos may have multiple interfaces (e.g., `BookingReader` + `BookingWriter`) if CQRS-like separation benefits testability.

### D3 — Domain entities add behavior, not wrappers
- **Decision**: Move current model structs to `internal/domain/entity/` and ADD methods. No new wrapper types.
- **Rationale**: The structs are already correct. Adding `CanCancel()`, `IsOverlapping()`, `ValidTransitions()` on the entity avoids anemic domain.
- **Consequence**: `internal/model/` becomes a deprecated re-export or is deleted after Phase 3.

### D4 — SemanticError stays but moves to domain
- **Decision**: `SemanticError` struct moves to `internal/domain/errors.go`. Same shape, same behavior.
- **Rationale**: Business error codes (`ErrCodeBookingOverlap`, `ErrCodeSlotInPast`) are domain concepts.
- **Consequence**: `internal/repository/errors.go` is deleted in Phase 3.

### D5 — Domain services are stateless
- **Decision**: `internal/domain/service/` contains pure functions and stateless structs. No DB access, no state.
- **Rationale**: `CheckAvailability` in domain service receives repository interfaces as arguments, not as struct fields.
- **Consequence**: The domain service is purely testable with mock repos.

### D6 — Use cases are single-file structs
- **Decision**: Each use case in `internal/application/usecase/` is one file, one exported struct, one `Execute` method.
- **Rationale**: Avoid over-engineering. Go doesn't need `CreateBookingUseCaseInput` as a separate interface.
- **Consequence**: Simple. Testable. Easy to navigate.

### D7 — Use case receives concrete DTOs, returns concrete results
- **Decision**: `CreateBookingUseCase.Execute(ctx, dto.CreateBookingInput)` returns `(*dto.CreateBookingResult, error)`.
- **Rationale**: Explicit contract. No `any` types. IDE-friendly.
- **Consequence**: DTO package grows but each file is small.

### D8 — Auth helpers move to `internal/auth/`
- **Decision**: `requireCaller`, `requireClientMatch` move to `internal/auth/` as exported functions.
- **Rationale**: Auth is a cross-cutting concern, not a repository concern.
- **Consequence**: Repos import `internal/auth/` for caller extraction. Use cases import `internal/auth/` for authorization decisions.

### D9 — No DI framework
- **Decision**: Manual DI in `cmd/mcp-server/main.go` via constructor injection.
- **Rationale**: Go doesn't need DI containers. 10 repos × a few services is manageable.
- **Consequence**: Slightly more code in `main()` but zero framework dependency.

### D10 — Migration is additive, existing tests keep passing
- **Decision**: Each phase must leave `go test -race ./...` passing.
- **Rationale**: No big-bang. PRs stay reviewable ( <600 lines).
- **Consequence**: Phases 1-2 are pure adds. Phase 3 does the rewire.

## Migration Phases — Detailed

### Phase 1: Extract Domain Layer

**Files to create (additive)**:

```
internal/domain/
├── entity/
│   ├── booking.go              # move + enrich from model/booking.go
│   ├── client.go               # move + enrich from model/client.go
│   ├── professional.go         # move + enrich
│   ├── service.go              # move + enrich
│   ├── business_profile.go     # move + enrich
│   ├── business_hours_exception.go  # move + enrich
│   ├── pending_alert.go        # move + enrich
│   ├── schedule.go             # move + enrich
│   └── account.go              # move + enrich
├── repository/
│   ├── bookings.go             # BookingRepository interface
│   ├── clients.go              # ClientRepository interface
│   ├── professionals.go
│   ├── services.go
│   ├── business_profile.go
│   ├── business_hours_exception.go
│   ├── pending_alerts.go
│   ├── schedules.go
│   └── accounts.go
├── service/
│   └── availability.go         # CheckAvailability as domain service
└── errors.go                   # SemanticError + sentinel errors + error codes
```

**Key interface shape** (example):

```go
// internal/domain/repository/bookings.go
type BookingRepository interface {
    FindByID(ctx context.Context, id string) (*entity.Booking, error)
    Save(ctx context.Context, booking *entity.Booking) error
    Update(ctx context.Context, booking *entity.Booking) error
    FindOverlapping(ctx context.Context, professionalID string, start, end time.Time) (*entity.Booking, error)
}
```

**Entity enrichment** (example):

```go
// internal/domain/entity/booking.go
func (b *Booking) CanCancel() bool {
    return b.Status == BookingStatusPending || b.Status == BookingStatusConfirmed
}

func (b *Booking) IsOverlapping(other *Booking) bool {
    return b.ProfessionalID == other.ProfessionalID &&
        b.StartDatetime.Before(other.EndDatetime) &&
        b.EndDatetime.After(other.StartDatetime)
}
```

### Phase 2: Application Layer

**Files to create**:

```
internal/application/
├── usecase/
│   ├── create_booking.go
│   ├── cancel_booking.go
│   ├── reschedule_booking.go
│   ├── check_availability.go
│   └── get_booking.go
└── dto/
    ├── create_booking.go
    ├── cancel_booking.go
    ├── reschedule_booking.go
    ├── check_availability.go
    └── get_booking.go
```

**Use case pattern**:

```go
// internal/application/usecase/create_booking.go
type CreateBookingUseCase struct {
    bookingRepo domain.BookingRepository
    serviceRepo domain.ServiceRepository
}

func NewCreateBookingUseCase(
    bookingRepo domain.BookingRepository,
    serviceRepo domain.ServiceRepository,
) *CreateBookingUseCase {
    return &CreateBookingUseCase{
        bookingRepo: bookingRepo,
        serviceRepo: serviceRepo,
    }
}

func (uc *CreateBookingUseCase) Execute(ctx context.Context, input dto.CreateBookingInput) (*dto.CreateBookingResult, error) {
    // 1. Extract caller from context via auth.FromContext(ctx)
    // 2. Resolve service duration via serviceRepo
    // 3. Check availability (optional)
    // 4. Create booking entity
    // 5. Save via bookingRepo
    // 6. Return result DTO
}
```

> **Note**: Auth is accessed via `auth.FromContext(ctx) *auth.Caller` directly in use cases, not through a separate `auth.Checker` interface — the current `internal/auth/` package already exposes `FromContext`. No new auth abstraction is needed.
```

### Datetime type migration

Current model structs store datetimes as `string` fields (e.g., `Booking.StartDatetime string`). Domain entities should use `time.Time` for type safety.

**Per-file approach** (no bulk datetime refactor): As each entity file is created in Phase 1, convert its datetime fields from `string` to `time.Time`. The existing model files remain untouched (`string`). During Phase 3 migration, the `string ↔ time.Time` conversion happens at the repository SQL-scan boundary.

Exceptions: `BusinessHoursException.Date`, `BusinessProfile.OpenTime`/`CloseTime`, and `Schedule.StartTime`/`EndTime` are time-of-day or date-only fields — keep as `string` with format validation in the entity.

### Phase 3: Refactor Repositories

**Changes to existing files**:

| File | Action |
|------|--------|
| `internal/repository/bookings.go` | Implement `domain.BookingRepository`; remove `CheckAvailability` |
| `internal/repository/errors.go` | DELETE — errors moved to `internal/domain/errors.go` |
| `internal/repository/auth_helpers.go` | DELETE — moved to `internal/auth/` |
| `internal/repository/*.go` | Add `implements domain.XxxRepository` interface conformance |
| `internal/model/*.go` | DELETE or alias to `internal/domain/entity/` |

**Repo pattern after refactor**:

```go
// internal/repository/bookings.go
type BookingsRepo struct {
    db *sql.DB
}

func NewBookingsRepo(db *sql.DB) *BookingsRepo {
    return &BookingsRepo{db: db}
}

// Ensure interface compliance
var _ domain.BookingRepository = (*BookingsRepo)(nil)

func (r *BookingsRepo) FindByID(ctx context.Context, id string) (*entity.Booking, error) {
    // same SQL as current GetBooking
}

func (r *BookingsRepo) Save(ctx context.Context, b *entity.Booking) error {
    // same SQL as current CreateBooking
}
```

### Phase 4: Wire DI

```go
// cmd/mcp-server/main.go
func main() {
    db, err := db.NewDatabase(ctx, dsn)
    // ...
    bookingRepo := repository.NewBookingsRepo(db)
    serviceRepo := repository.NewServicesRepo(db)
    // ...

    createBooking := usecase.NewCreateBookingUseCase(bookingRepo, serviceRepo)

    // Wire MCP handler with createBooking.Execute
}
```

### Phase 5: Transport (future)

When MCP handlers are built in `internal/mcp/`, they receive use cases, not repos.

## Test Strategy

| Layer | Test approach | Runtime dep |
|-------|---------------|-------------|
| `domain/entity/` | Pure unit tests, no mocks | None |
| `domain/service/` | Unit tests with mock repo (via domain interfaces) | None |
| `domain/repository/` | Interface compliance tests | None |
| `application/usecase/` | Unit tests with mock repos | None |
| `repository/` (SQL) | SQL mock tests (sqlmock) — keep existing | sqlmock |
| `mcp/` | Integration tests | Full server |

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| P3 breaks existing tests | Medium | High | Run `go test -race ./...` before every commit |
| Scope creep (over-abstract) | Medium | Medium | Target pragmatic boundaries, no DI framework |
| Long migration (5 phases) | High | Low | Each phase is independently shippable |
| Domain/service overlaps with use case | Low | Low | Domain service = business rule; use case = orchestration |

## Effort Estimate

| Phase | Files | Est. LOC | Est. sessions | Risk |
|-------|-------|----------|---------------|------|
| P1 — Domain layer | ~20 new | ~800 | 1-2 | None (additive) |
| P2 — Application | ~12 new | ~600 | 1 | None (additive) |
| P3 — Refactor repos | ~15 modified | ~500 changed | 2-3 | Medium |
| P4 — Wire DI | ~2 modified | ~100 | 1 | Low |
| P5 — Transport | Future | Future | Future | Future |

**Total**: ~5-7 sessions, ~2000 LOC new/changed

# Spec: Clean Architecture — Layer Contracts

> **Change**: refactor/clean-architecture
> **Domain**: architecture
> **Status**: Specified
> **Date**: 2026-07-29

## Overview

Defines the contractual boundaries between architectural layers after the refactor. Each phase of the migration must satisfy these contracts before it is considered complete.

## Layer Contracts

### C1 — Domain Layer (`internal/domain/`)

**Zero dependencies rule**: `internal/domain/` MUST NOT import any package outside itself. No imports of `database/sql`, `internal/repository/`, `internal/auth/`, `internal/db/`, `net/http`, or any external transport library. Only standard library packages (`time`, `context`, `errors`, `fmt`) and `internal/domain/` sub-packages are allowed.

**Sub-packages**:

| Package | Responsibility | Import rule |
|---------|---------------|-------------|
| `domain/entity/` | Business entities with behavior | Must not import domain/repository/ |
| `domain/repository/` | Repository interfaces only | Must not import domain/entity/ (interface methods may reference entities by import) |
| `domain/service/` | Stateless business logic | May import domain/entity/ and domain/repository/ (interfaces only) |
| `domain/errors.go` | Shared error types | No sub-package imports |

**Entity contract**:
- Each entity MUST be a struct with exported fields AND at least one business method (e.g., `CanCancel()`, `IsOverlapping()`)
- **Exception**: Pure value objects that carry data without invariants (`Client`, `Account`) MAY omit business methods. If no invariant or behavior exists on the data, wrapping in a method would be noise. A business method MUST be added if ANY rule depends on the entity's fields (e.g., `Client.HasActiveMembership()`).
- Entities MUST NOT have `json:` or `db:` struct tags — those belong in DTOs
- Entities MUST NOT have `*sql.DB`, `*sql.Rows`, or any database type as fields
- Validation logic that depends ONLY on the entity's own state belongs as a method on the entity

**Repository interface contract**:
- Each repository interface MUST be defined in terms of domain entities and domain errors only
- Interface methods MUST accept and return domain types, not SQL types
- Interface methods SHOULD favor simple signatures: `FindByID(ctx, id)`, `Save(ctx, entity)`, `Update(ctx, entity)`
- Interface MUST NOT expose pagination, sorting, or filtering unless required by a use case

**Domain service contract**:
- Services MUST be stateless (no struct fields beyond zero-value configuration)
- Business logic that spans multiple entities OR requires repository calls belongs in a domain service
- Services MUST receive repository interfaces as method arguments, not as constructor-injected fields
- Services MUST NOT import `database/sql` or any infrastructure package

**Domain errors contract**:
- `SemanticError` with `Code`, `Message`, `Cause` fields — same shape as current, defined in `domain/errors.go`
- All sentinel errors (`ErrNotFound`, `ErrConflict`, `ErrInvalidInput`, `ErrUnauthenticated`) in `domain/errors.go`
- All error codes (`ErrCodeBookingOverlap`, `ErrCodeSlotInPast`, etc.) in `domain/errors.go`
- `internal/repository/errors.go` MUST be deleted in Phase 3
- **Consolidation required**: Two `ErrUnauthenticated` exist — `internal/repository/auth_helpers.go:14` ("caller not authenticated") and `internal/auth/resolver.go:12` ("unauthenticated"). The domain `errors.go` MUST pick ONE canonical message. Also, the `ErrCode` type from `internal/repository/errors.go` and the one in `domain/errors.go` will be distinct types — Phase 3 MUST update all consumers to use the domain type, with explicit conversion at the repo boundary.

### C2 — Application Layer (`internal/application/`)

**Dependency rule**: `internal/application/` MUST ONLY import `internal/domain/` and `context`. It MUST NOT import `internal/repository/`, `database/sql`, `net/http`, or any infrastructure package.

**Sub-packages**:

| Package | Responsibility | Import rule |
|---------|---------------|-------------|
| `application/usecase/` | Business flow orchestration | May import domain/ (entities, interfaces, errors) |
| `application/dto/` | Request/response types | May import domain/entity/ |

**Use case contract**:
- Each use case is one file, one exported struct with constructor, one `Execute(ctx, input) (*Result, error)` method
- Use cases receive domain repository interfaces as constructor arguments (DI)
- Use cases orchestrate: auth check → domain service call → repo call → return DTO
- Use cases MUST NOT contain raw SQL
- Use cases MUST NOT contain business rules that belong in domain entities or services
- Use cases MUST return DTOs, not domain entities

**DTO contract**:
- DTOs may duplicate entity fields — this is intentional (decoupling)
- DTOs MAY have `json:` tags for API serialization
- DTOs MUST NOT embed domain entities
- Input DTOs MAY have validation tags or methods
- Result DTOs SHOULD be concrete structs, not `any` or `interface{}`

### C3 — Repository Layer (`internal/repository/`)

**After Phase 3 only**: Each repo struct MUST satisfy the corresponding `domain/repository/` interface.

```go
var _ domain.BookingRepository = (*BookingsRepo)(nil)
```

**Contracts**:
- Repos receive `*sql.DB` via constructor — same as current
- Repos handle SQL mapping: domain entity ↔ SQL row
- Repos MUST NOT contain business logic that belongs in domain entities or services
- After Phase 3: `errors.go` deleted (domain errors moved), `auth_helpers.go` deleted (moved to `internal/auth/`)
- `model/` (if still present) must not be imported by new use cases

### C4 — Auth Layer (`internal/auth/`)

**After Phase 3 only**:
- `requireCaller(ctx)` and `requireClientMatch(ctx, clientID, professionalID)` exported from `internal/auth/`
- Auth middleware and checker remain as-is
- **Auth gains one new dependency**: `requireClientMatch` returns `domain.SemanticError` with `domain.ErrCodeUnauthenticated`, so `internal/auth/` MUST import `internal/domain/errors`. This is intentional — domain errors are a stable cross-cutting concern, and the dependency is one-directional (domain ← auth).

### C5 — Dependency Injection (`cmd/`)

**After Phase 4 only**:
- `cmd/` is the ONLY package that knows about all concrete implementations
- `cmd/mcp-server/main.go` constructs repos, services, use cases, and wires them
- No `init()` functions for DI
- No global state for DI

## Migration Phase Criteria

### Phase 1 Complete Checklist
- [ ] `internal/domain/entity/` exists with all 9 entity files, each with at least one business method
- [ ] `internal/domain/repository/` exists with all 9 repository interfaces
- [ ] `internal/domain/errors.go` exists with SemanticError + sentinel errors + error codes
- [ ] `internal/domain/service/availability.go` exists with CheckAvailability as domain service
- [ ] `internal/model/` still exists and compiles (backward compat)
- [ ] `go build ./...` passes
- [ ] `go test -race ./...` passes (same or higher count — P1.4d adds new tests)

### Phase 2 Complete Checklist
- [ ] `internal/application/usecase/` exists with all 5 use cases
- [ ] `internal/application/dto/` exists with request/response types
- [ ] No infrastructure imports in `internal/application/`
- [ ] `go build ./...` passes
- [ ] `go test -race ./...` passes

### Phase 3 Complete Checklist
- [ ] Every `internal/repository/` file (except errors.go and auth_helpers.go) implements its domain interface
- [ ] `internal/repository/errors.go` deleted
- [ ] `internal/repository/auth_helpers.go` deleted
- [ ] `requireCaller` and `requireClientMatch` exist in `internal/auth/`
- [ ] `internal/model/` deleted (consumers updated to use `internal/domain/entity/`)
- [ ] Business logic removed from repo methods (delegated to domain service or entity)
- [ ] `go build ./...` passes
- [ ] `go test -race ./...` passes

### Phase 4 Complete Checklist
- [ ] `cmd/mcp-server/main.go` wires everything via DI
- [ ] No package outside `cmd/` imports both a domain interface and its concrete implementation
- [ ] `go build ./...` passes
- [ ] `go test -race ./...` passes

### Phase 5 (Future) — not gated

## Migration Rules

### R1 — Additive first
Phases 1 and 2 MUST NOT modify any existing file. They only create new files. This guarantees zero regression risk.

### R2 — Tests pass at every commit
Every commit in every phase must pass `go test -race ./...`. No exceptions.

### R3 — No orphaned code
When a file is deleted (Phase 3), ALL references to it must be updated first. The build must pass without the deleted file.

### R4 — PRs under review budget
Each phase MUST be split into PRs under the review budget (600 lines). See `tasks.md` (Sub-PR Plan section) for the per-phase split strategy.

### R5 — Domain never imports infra
`go vet ./internal/domain/...` must not show any unexpected imports. A custom `go vet` check or `grep -r 'database/sql' internal/domain/` must return empty.

## Requirements

> **Provenance**: the following requirements were ADDED by change `feat-mcp-transport` (merged to main via PR #46 `98d7be3` + PR #47 `bb86228`, archived 2026-08-19).

### REQ-ARCH-INTMCP-001 — New adapter layer internal/mcp/

A new adapter layer `internal/mcp/` MUST be added to the Hexagonal model. Its role is the MCP transport: Streamable HTTP server, JSON-RPC 2.0 framing, tool registration, request/response mapping.

#### Scenario: Layer exists
- GIVEN the project structure
- WHEN reviewed
- THEN `internal/mcp/` MUST exist with at least one `.go` file implementing the MCP transport

### REQ-ARCH-INTMCP-002 — Composition root remains cmd/

`cmd/mcp-server/main.go` MUST remain the only composition root. It wires `internal/mcp/` adapters to `internal/application/usecase/` ports.

#### Scenario: Wiring in cmd/
- GIVEN the composition root
- WHEN reviewed
- THEN `cmd/mcp-server/main.go` MUST construct the MCP transport and inject use case interfaces

### REQ-ARCH-INTMCP-003 — Consumer interfaces declared in internal/mcp/

`internal/mcp/` MUST declare the consumer interfaces it needs (per data-access C5). The transport MUST NOT import `internal/repository/` directly.

#### Scenario: No direct repository import
- GIVEN the source of `internal/mcp/`
- WHEN imports are reviewed
- THEN `internal/repository` MUST NOT appear in any production `.go` file

### REQ-ARCH-INTMCP-004 — Adapter conventions

The new layer MUST follow existing adapter conventions: structured `log/slog`, Spanish error messages via `*domain.SemanticError`, `context.Context` propagation, `defer` for cleanup, `errors.Is` for sentinel checks.

#### Scenario: Structured logging used
- GIVEN the source of `internal/mcp/`
- WHEN error handling is reviewed
- THEN `*slog.Logger` MUST be used for all logging
- AND errors MUST be wrapped with `fmt.Errorf("...: %w", err)`

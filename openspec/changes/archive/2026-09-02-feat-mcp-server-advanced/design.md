# Design: feat-mcp-server-advanced (Fase 3)

## Technical Approach

App-layer only, no DDL, `schema_version` stays 1. Widen 3 domain interfaces (`ClientsRepo.SearchFTS`, `ServicesRepo.SearchFTS`, `BookingsRepo.AggregateByClient`) — concrete `SearchFTS` already at `clients.go:310`/`services.go:143`. Add 2 pending-alerts methods via port. 5 use cases + 5 MCP tools (ports→Config→registrar→ToolRBAC), registry 6→11 (REQ-MT-005/015).

## Architecture Decisions

| # | Decision | Choice | Alternatives rejected | Rationale |
|---|----------|--------|----------------------|-----------|
| A1 | Staff FTS scoping | `applyClientsAuthFilter` adds `RoleStaff`: `AND id IN (SELECT client_id FROM bookings WHERE professional_id=?)`, nil PID→forbidden | 403 blanket; separate filter | Reuses tested filter; no status predicate matches spec linkage |
| A2 | Alert placement | Post-mutation in create/cancel/reschedule via `AlertLifecycleStore{InsertForBooking, CancelByBookingID}`, `RequireCaller` only, `Save` untouched | Relax `Save`; emit from handler | Staff must emit (REQ-PA-LIFE-001); validator precedent; log on fail, booking succeeds |
| A3 | Loyalty aggregation | `BookingsRepo.AggregateByClient`: JOIN clients, `status!='cancelled'`, `start>=? AND start<?`, `GROUP BY`, `ORDER BY COUNT(*) DESC, c.name ASC LIMIT ?`; period+clamp in use case | In-memory group; materialized view | Single statement, deterministic tie-break, [start,end) maps to params |
| A4 | D1 dropped | No UNIQUE index; duplicates tolerated | Add index now | User decision; trivial rollback (revert PRs) |
| A5 | RBAC split | No `ToolRBAC` for search (repo scoping, `get_booking` precedent); `{owner,admin}` for `get_pending_alerts`, `mark_alert_as_sent`, `get_loyalty_report` | Coarse entries | Matches spec roles, PII gate for phones |

## Data Flow

```
Hermes ──▶ AuthMiddleware ──▶ handler ──▶ UseCase.Execute
                    ┌────────┼─────────────┐
                    ▼        ▼             ▼
             SearchFTS  AggregateByClient  InsertForBooking/
                                         CancelByBookingID
                    └────────┴─────────────┘
                             ▼
                   SQLite (WAL, busy_timeout=5000, ?)
create: commit → InsertForBooking → fail? log only
cancel/reschedule: mutate → CancelByBookingID → (reschedule +Insert)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/domain/repository/clients.go` | Modify | Add `SearchFTS` |
| `internal/domain/repository/services.go` | Modify | Add `SearchFTS` |
| `internal/domain/repository/bookings.go` | Modify | Add `AggregateByClient` |
| `internal/repository/clients.go` | Modify | `RoleStaff` branch in filter |
| `internal/repository/pending_alerts.go` | Modify | `InsertForBooking`, `CancelByBookingID` (pending-only) |
| `internal/repository/bookings.go` | Modify | Implement `AggregateByClient` |
| `internal/application/dto/search.go` | Create | Search + loyalty DTOs |
| `internal/application/dto/alerts.go` | Create | Alert DTOs + inputs |
| `internal/application/usecase/search_clients_advanced.go` | Create | Delegates to `SearchFTS` |
| `internal/application/usecase/search_services_advanced.go` | Create | `RequireRole(owner,admin)` → `SearchFTS` |
| `internal/application/usecase/get_pending_alerts.go` | Create | `RequireRole` + `FindPending(now)` |
| `internal/application/usecase/mark_alert_as_sent.go` | Create | `RequireRole` + `MarkAsSent` |
| `internal/application/usecase/get_loyalty_report.go` | Create | Period parse, clamp [1,100] d10, aggregate |
| `internal/application/usecase/alerts.go` | Create | Port + Paso-5 builder + log helper |
| `internal/application/usecase/{create,cancel,reschedule}_booking.go` | Modify | Inject store; emit/cancel alerts |
| `internal/mcp/ports.go` | Modify | 5 ports |
| `internal/mcp/config.go` | Modify | 5 fields |
| `internal/mcp/tools_search.go` | Create | Register search tools |
| `internal/mcp/tools_alerts.go` | Create | Register alert+loyalty tools |
| `cmd/mcp-server/main.go` | Modify | Wire repos/use cases; 3 RBAC entries |

## Interfaces / Contracts

```go
SearchFTS(ctx context.Context, query string) ([]*entity.Client, error) // bm25 ASC
AggregateByClient(ctx context.Context, start, end time.Time, limit int) ([]ClientBookingCount, error)
InsertForBooking(ctx context.Context, a *entity.PendingAlert) error // RequireCaller
CancelByBookingID(ctx context.Context, bookingID string) error // pending-only
// DTOs per REQ-MT-015: ClientSearchEntry, ServiceSearchEntry, PendingAlertView, LoyaltyReportEntry
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Role matrix, period+clamp, Paso-5 msg, cancel idempotency | Table-driven + mocks |
| Integration | create→alert, cancel/reschedule lifecycle, loyalty windows, FTS scoping | In-memory SQLite, `-race` |
| E2E | `tools/list`=11, invalid inputs, role rejections | server_integration_test style |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, or process-integration boundary.

## Migration / Rollout

No migration. Rollback: revert PRs; tools vanish; pending rows inert.

## Open Questions

- [ ] Staff `FindByID` also becomes bookings-scoped: accept or split filters?
- [ ] Reschedule alert `scheduled_datetime` = mutation time, msg carries new start — confirm?

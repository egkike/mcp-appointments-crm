# Tasks: feat-mcp-server-advanced (Fase 3)

## Review Workload Forecast

Estimated changed lines: ~1700–2100 / 19 files

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

**ask-on-risk**: High ⇒ user approved stacked-to-main; slices autonomous.

### Work Units

- Unit 1 → PR 1, caller-scoped FTS · test `go test -race ./...` · harness SQLite fixtures · rollback revert filter+tools
- Unit 2 → PR 2, alert lifecycle · test `go test -race -run Alert ./...` · harness mutation flows · rollback alerts inert
- Unit 3 → PR 3, loyalty report · test `go test -race -run 'Loyalty|Aggregate' ./...` · harness seeded windows/ties · rollback drop tool
- Unit 4 → PR 4, registry 11 + E2E · test `go test -v -race ./...` · harness server_integration_test · rollback revert wiring/RBAC

## Phase 1: FTS Search (PR 1) — REQ-CL-AUTH-004

- [x] 1.1 RED: staff-filter tests — linked returned (any status), unlinked excluded, nil professional_id forbidden, admin/owner all, client own row, caller-less rejected (REQ-CL-AUTH-004)
- [x] 1.2 GREEN: `RoleStaff` subquery branch in `repository/clients.go` (no status predicate); widen `ClientsRepo.SearchFTS`/`ServicesRepo.SearchFTS` in `domain/repository/`; integration: bm25 kept, no-match → empty
- [x] 1.3 Create `dto/search.go` (ClientSearchEntry, ServiceSearchEntry) + use cases `search_clients_advanced.go` (repo-scoped, A5), `search_services_advanced.go` (`RequireRole(owner,admin)`, forbidden error); reuse `validateFTSQuery`; unit-test rejection
- [x] 1.4 Add 2 ports (`mcp/ports.go`) + config fields; create `mcp/tools_search.go` (empty `query_text` → invalid input); wire `cmd/mcp-server/main.go` (search: no RBAC entry); e2e: transport role-neutral

## Phase 2: Alert Lifecycle (PR 2) — REQ-PA-LIFE-001/CANCEL-002

- [x] 2.1 RED: `CancelByBookingID` tests — pending-only transition, sent/cancelled untouched, no-match nil, idempotent (REQ-PA-CANCEL-002)
- [x] 2.2 GREEN: `InsertForBooking` (RequireCaller) + pending-only `CancelByBookingID` in `repository/pending_alerts.go`; create `usecase/alerts.go` (AlertLifecycleStore port, Paso-5 builder UTC, log-don't-fail helper); unit-test message
- [x] 2.3 Create `dto/alerts.go`; use cases `get_pending_alerts.go` (`RequireRole(owner,admin)`, oldest-first) + `mark_alert_as_sent.go`; unit tests
- [x] 2.4 Inject store into create/cancel/reschedule_booking post-commit: insert `confirmation_requested` / cancel / cancel+insert; alert-save failure logs, booking succeeds (REQ-PA-LIFE-001); integration -race: lifecycle+failure path
- [x] 2.5 Add 2 ports/config fields; register tools in `mcp/tools_alerts.go`; add `ToolRBAC` `{owner,admin}` entries in main.go; e2e: staff/client → `-32001` Spanish error

## Phase 3: Loyalty Report (PR 3) — REQ-BK-AGG-001, REQ-LR-001..004

- [x] 3.1 RED: `AggregateByClient` integration tests — cancelled excluded, `[start,end)` bounds, count DESC + name-ASC tie-break, LIMIT caps (REQ-BK-AGG-001)
- [x] 3.2 GREEN: interface in `domain/repository/bookings.go`; JOIN+GROUP BY impl in `repository/bookings.go`, parameterized `?` only
- [x] 3.3 Create `get_loyalty_report.go` + tests: period enum `last_week|last_month|last_quarter|last_year|all_time`, omitted → last_month, now-UTC windows, invalid → `ErrInvalidInput` (REQ-LR-001); `top_n` clamp [1,100] default 10 (REQ-LR-002)
- [x] 3.4 Add LoyaltyReportEntry to `dto/search.go`; port/config field; register `{owner,admin}` noting phone PII; wire main.go; integration: five windows, role rejections, empty → [] (REQ-LR-003/004)

## Phase 4: Wiring & E2E (PR 4)

- [x] 4.1 E2E: `tools/list` = 11 descriptors; `-32001` role matrix on gated trio; searches admit all authenticated; invalid inputs (empty `query_text`, missing `alert_id`) — REQ-MT-005/015
- [x] 4.2 Gate: `go fmt`/`vet`/`golangci-lint run`; `go build -o /dev/null ./...`; `go test -v -race ./...`
- [x] 4.3 Update docs listing 6 tools, if any; pin allowlist `confirmation_requested`; no DDL, no migration

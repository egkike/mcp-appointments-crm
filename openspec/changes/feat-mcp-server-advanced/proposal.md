# Proposal: feat-mcp-server-advanced (Fase 3)

## Intent

Hermes cannot keyword-search clients/services, consume confirmation alerts, or pull loyalty reports. FTS5 infra and `pending_alerts` storage are live, but domain interfaces lack `SearchFTS`, alert generation unwired, loyalty report missing (RF3/RF7/RF8).

## Scope

### In Scope

- Widen domain interfaces: `ClientsRepo.SearchFTS`, `ServicesRepo.SearchFTS`.
- Staff-scoped client FTS (decision 1): staff scoped by repo filter (own clients via bookings); client own row; admin/owner all.
- New repo methods: `PendingAlertsRepo.CancelByBookingID` (pending-only), `BookingsRepo.AggregateByClient` (SQL `GROUP BY`, no cancelled).
- 5 use cases + DTOs: SearchClientsAdvanced, SearchServicesAdvanced, GetPendingAlerts, MarkAlertAsSent, GetLoyaltyReport.
- Alert wiring (B1): create inserts `confirmation_requested`; cancel cancels pending; reschedule cancels then inserts.
- 5 MCP tools: ports, config, registration, `ToolRBAC`, composition root.

### Out of Scope

- `reminder_24h` and `loyalty_alert` generation; allowlist extension (decisions 2–3). Allowlist stays `confirmation_requested`; loyalty manual via future reports.
- UNIQUE dedup (D1 dropped); revisit if retries duplicate. No schema change.
- `update_business_profile`, TUI menu, service templates, installation docs, bookings_archive, materialized view.

## Capabilities

### New Capabilities

- `loyalty-report`: period enum `last_week` to `all_time` (5 values; window from now UTC; invalid rejected), `top_n` clamp [1,100] default 10, `booking_count DESC`, no cancelled, phone PII.

### Modified Capabilities

- `mcp-transport`: REQ-MT-005/015 — registry grows 6 to 11 tools, descriptors, RBAC.
- `clients`: REQ-CL-AUTH-004 — staff forbidden becomes bookings-scoped (decision 1).
- `pending-alerts` + `bookings`: lifecycle wiring (create inserts, cancel/reschedule cancel + re-insert), `CancelByBookingID`, Fase 3 allowlist.

## Approach

A1+B1+C1; D1 dropped. Reuse `validateFTSQuery`. Alerts emitted post-mutation in booking use cases (§3.7.13 template); alert-save failure logs, never fails booking. Tools admit any authenticated caller; repo enforces scoping (`get_booking` precedent). Staff-forbidden FTS overridden: staff search own clients (decision 1). Loyalty: one SQL aggregation, period parsed in use case.

## Affected Areas

- `internal/domain/repository/` + `internal/repository/` — widen clients/services interfaces, staff filter, new methods.
- `internal/application/` — 5 DTOs/use cases; alert wiring in 3 booking use cases.
- `internal/mcp/` — ports, config, `server.go`, tool registrars.
- `cmd/mcp-server/main.go` — composition root, `ToolRBAC`.

## Risks

- Exceeds 400-line budget (High): chained PRs — FTS, alerts, loyalty, wiring.
- Alert save fails post-commit (Low): log, return success.
- Retry duplicates alerts (Low): unique index if observed.
- Staff FTS scope leak (Medium): role-based tests.

## Rollback Plan

No schema changes. Revert chained PRs; tools vanish from `tools/list`; pending rows inert, manually cancellable.

## Dependencies

Live FTS5 tables/triggers, `pending_alerts` schema; PRD §3.7–§3.8.

## Success Criteria

- [ ] `tools/list` returns 11 tools.
- [ ] FTS scoping correct per role; invalid query → semantic error, never SQL.
- [ ] Create inserts one pending alert; cancel cancels pending; reschedule cancels then inserts.
- [ ] Loyalty: period enforced, `top_n` clamped, ranked, no cancelled.
- [ ] Race-detector tests green; 400-line slices.

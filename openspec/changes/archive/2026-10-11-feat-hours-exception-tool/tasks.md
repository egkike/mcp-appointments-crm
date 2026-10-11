# Tasks: feat-hours-exception-tool

> **Change:** feat-hours-exception-tool
> **Status:** tasks drafted (spec phase) — ready for apply planning
> **Inputs:** `proposal.md`, `design.md`, `specs/mcp-transport/spec.md` (MODIFIED delta), capability delta (ADDED requirements)
> **Hard rule:** every work unit ≤ ~100 changed lines (review band). Test-first for all Go units (`openspec/config.yaml` `apply.tdd: true`).

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~650–850 (≈250 impl + ≈400 tests + ≈100 docs/spec) |
| 400-line budget risk | **High** (`openspec/config.yaml` `review_budget_lines: 400`) |
| Chained PRs recommended | **Yes** |
| Suggested split | PR1 use cases + DTOs → PR2 transport tools + RBAC + composition wiring → PR3 integration tests + docs/spec reconciliation |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main (each PR targets `main` from its own feature branch, merged in order) |

Each work unit below is sized to fit a ≤ ~100-line review slice; a unit that grows past that MUST be split before apply. `Decision needed before apply: Yes` (open questions OQ-1/OQ-2 in `design.md` §9). `Chained PRs recommended: Yes`. `Chain strategy: stacked-to-main`. `400-line budget risk: High`.

---

## Phase 1 — Use cases + DTOs (PR1)

> Depends on nothing. No external behavior change (no tool exists yet). Reuses `BusinessHoursExceptionRepo` as-is.

### TASK-1.1 — RED: use-case tests for create

- [ ] Add failing tests in `internal/application/usecase` covering: owner creates a closed exception; owner creates an open exception with a custom window; admin/staff/client rejected with `ErrCodeForbidden`; `is_closed=true` + times rejected; `is_closed=false` without times rejected; `open_time >= close_time` rejected; malformed/calendar-invalid date rejected; duplicate date → `ErrCodeConflict`; every failure is a `*domain.SemanticError` with a Spanish message; no write happens before the gate passes
- [ ] Record the observed failure output
- **Edit surface:** `internal/application/usecase/hours_exception_test.go` (new)

### TASK-1.2 — GREEN: create use case + DTO

- [ ] Add `dto.CreateHoursExceptionInput` (`Caller auth.Caller` + `ExceptionDate string`, `IsClosed bool`, `OpenTime/CloseTime/Reason *string`) to `internal/application/dto/maintenance.go` with cross-layer doc comments
- [ ] Create `internal/application/usecase/create_hours_exception.go` following `create_service.go`: `WithCaller` → `RequireRole(RoleOwner)` → build entity → `repo.Create` → map errors (pass through `*domain.SemanticError` first, then `ErrInvalidInput`/`ErrConflict`) → `repo.Get(date)` read-back → `logMaintenanceMutation(..., "create_hours_exception", "business_hours_exception", id, "create")`
- [ ] Run TASK-1.1 green
- **Edit surface:** `internal/application/usecase/create_hours_exception.go` (new), `internal/application/dto/maintenance.go`

### TASK-1.3 — GREEN: delete use case + DTO

- [ ] RED first: tests for owner delete success, unknown id → `ErrCodeNotFound` Spanish, forbidden roles
- [ ] Add `dto.DeleteHoursExceptionInput` + `dto.DeleteHoursExceptionResult` to `internal/application/dto/maintenance.go`
- [ ] Create `internal/application/usecase/delete_hours_exception.go` following `delete_schedule.go`: owner gate, `repo.Delete(ctx, id)`, map `ErrNotFound`, `logMaintenanceMutation(..., "delete")`, return result
- [ ] Run the delete tests green
- **Edit surface:** `internal/application/usecase/delete_hours_exception.go` (new), `internal/application/usecase/hours_exception_test.go`, `internal/application/dto/maintenance.go`

### TASK-1.4 — GREEN: list use case + DTO

- [ ] RED first: tests for inclusive range pass-through (from/to forwarded verbatim), empty result is `[]` not an error, malformed bound → `ErrCodeInvalidInput` Spanish, forbidden roles
- [ ] Add `dto.ListHoursExceptionsInput` to `internal/application/dto/maintenance.go`
- [ ] Create `internal/application/usecase/list_hours_exceptions.go`: owner gate, parse `from`/`to` (`YYYY-MM-DD`) → `repo.List(ctx, from, to)`; **no audit**
- [ ] Run the list tests green
- [ ] **Gate:** focused `go test -race ./internal/application/...` + `go build -o /dev/null ./...`
- **Edit surface:** `internal/application/usecase/list_hours_exceptions.go` (new), `internal/application/usecase/hours_exception_test.go`, `internal/application/dto/maintenance.go`

---

## Phase 2 — Transport tools + RBAC + composition wiring (PR2)

> Changes the external MCP surface. Order matters: ports/config, then handlers, then `ToolRBAC` + DI, then the count literal.

### TASK-2.1 — RED: transport tests for the three tools

- [ ] Add failing tests in `internal/mcp`: `tools/list` contains `create_hours_exception`, `delete_hours_exception`, `list_hours_exceptions` and the count is 23; create input decoding (required `exception_date`/`is_closed`, optional times/reason); output `hoursExceptionOut` shape; delete output `{exception_id, status: "deleted"}`; list output ordered; each semantic error surfaces as `-32002`
- [ ] Record the observed failure output
- **Edit surface:** `internal/mcp/tools_hours_exception_test.go` (new)

### TASK-2.2 — GREEN: ports, config fields, handlers

- [ ] Add `CreateHoursExceptionPort`, `DeleteHoursExceptionPort`, `ListHoursExceptionsPort` to `internal/mcp/ports.go` (consumer-side, `Execute` signature; "ToolRBAC entry: owner only" doc comment)
- [ ] Add the three fields to `internal/mcp/config.go` `Config`
- [ ] Create `internal/mcp/tools_hours_exception.go`: typed in/out structs with JSON tags, `hoursExceptionOut` mapping, `s.toolNames[name] = struct{}{}` per tool, `auth.RequireCaller` + `s.toMCPError` + nil-result fail-closed guard (`errNilMaintenanceEntity` pattern), registered from a `registerHoursExceptionTools()` call in the existing registration path
- [ ] Run TASK-2.1 green
- **Edit surface:** `internal/mcp/ports.go`, `internal/mcp/config.go`, `internal/mcp/tools_hours_exception.go` (new)

### TASK-2.3 — GREEN: composition root — `ToolRBAC`, DI, count literal

- [ ] Add three `auth.RoleOwner` rows to the `ToolRBAC` map in `cmd/mcp-server/main.go:417-436`
- [ ] Construct the three use cases from the existing `bizHoursExRepo` and `logger`, and pass them into `mcp.NewServer(mcp.Config{...})` (`cmd/mcp-server/main.go:459-487`)
- [ ] Update `expectedToolCount` 20 → 23 in `internal/mcp/test_helpers_test.go`
- [ ] Run the transport suite green (`TestServerToolCountTracksRegisteredTools` must reconcile)
- [ ] **Gate:** `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, focused `go test -race ./internal/mcp/... ./cmd/mcp-server/...`
- **Edit surface:** `cmd/mcp-server/main.go`, `internal/mcp/test_helpers_test.go`

---

## Phase 3 — Integration tests + docs/spec reconciliation (PR3)

### TASK-3.1 — Integration through the real mux

- [ ] Add an end-to-end test: owner creates a closing exception for a date → `check_availability` for that date now reports business-closed; `list_hours_exceptions` returns it; `delete_hours_exception` removes it → availability reverts
- [ ] Add the negative cases: staff/client rejected with `-32001` on all three tools; duplicate date conflict over the mux
- [ ] Assert the registered surface is 23 over the real mux
- [ ] **Gate:** `go test -v -race ./...`
- **Edit surface:** `internal/mcp/server_integration_hours_exception_test.go` (new)

### TASK-3.2 — Docs and spec reconciliation

- [ ] Update `README.md:22` tool count 20 → 23 and list the three new tools
- [ ] Update `docs/PRD.md` §7 Fase N with the delivered item (if the owner approves the count per OQ-1)
- [ ] Reconcile the stale base spec `openspec/specs/mcp-transport/spec.md` (19 → 20) by applying the archived `feat-whatsapp-bot` delta, then the 20 → 23 delta of this change (OQ-5)
- [ ] Update `odd/tasks/feat-hours-exception-tool.md` status to reflect the implemented surface
- [ ] **Gate:** structural readback (docs gate) — no test execution
- **Edit surface:** `README.md`, `docs/PRD.md`, `openspec/specs/mcp-transport/spec.md`, `odd/tasks/feat-hours-exception-tool.md`

---

## Phase 4 — Closing gates (per PR, not per work unit)

- [ ] `go fmt ./...` and `go vet ./...` clean; `golangci-lint run ./...` clean
- [ ] `go build -o /dev/null ./...` passes; `go test -v -race ./...` passes
- [ ] No layering violations: no repository change; no SQL outside `internal/repository`; wire JSON tags only in `internal/mcp`; RBAC enforced at both layers; no new dependency
- [ ] Structural readback / native review per the Verification & Review Protocol routing before commit; GGA hook passes; operator approval obtained (never `--no-verify`)
- [ ] Chained-PR slices stay within the ~400-line budget; each PR's CI is green on its own SHA before the next push

---

## Traceability

| Requirement | Tasks |
|---|---|
| Three owner-only management tools exist and are registered (count 20 → 23) | 2.1, 2.2, 2.3 |
| Owner-only RBAC at both layers (double enforcement) | 1.1, 1.2, 1.3, 1.4, 2.2, 2.3, 3.1 |
| Create validation (date / `is_closed`↔times / `HH:MM` / `open<close`) reused from the repository | 1.1, 1.2 |
| Duplicate date → Spanish conflict; malformed → Spanish invalid input | 1.1, 1.2, 2.1, 3.1 |
| Delete by id; unknown id → Spanish not found | 1.3, 2.1, 3.1 |
| List takes an inclusive `from`/`to` range, ordered, empty ≠ error | 1.4, 2.1, 3.1 |
| One audit record per mutation; none for list | 1.1, 1.2, 1.3, 1.4 |
| Only `*domain.SemanticError` reaches the client | 1.2, 1.3, 1.4, 2.1, 2.2 |
| No schema/repository change | all (verified at 4) |
| Read path honors a wire-created exception | 3.1 |
| Docs/spec reconciled | 3.2 |

# Tasks: feat-mcp-transport

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated production LOC | 848 |
| Estimated changed lines | 1383 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 (feature-branch-chain) |
| Delivery strategy | ask-on-risk |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

> **LOC methodology**: the forecast is **projected applied LOC** (production code + tests to be written), NOT current spec LOC — the six current spec files sum to 952 lines (numstat: 321+163+45+44+191+188) and are not part of the forecast. Per-file production estimates come from design §2, verified against existing code shapes; design §2 file estimates sum to 905 prod for `internal/mcp/` alone, and the PR breakdown below (848 prod total: PR 1 = 428, PR 2 = 420) is the authoritative per-PR figure after task-level trimming; design §9's 300–350 / 270–370 and proposal §8's 570–720 are earlier production-only ranges superseded here. Total changed lines (1383 = 848 prod + 520 test + 15 doc) drive the 400-line budget risk.

### PR Breakdown

| PR | Production | Tests | Other | Total changed |
|----|-----------|-------|-------|---------------|
| PR 1 | 428 | 180 | — | 608 |
| PR 2 | 420 | 340 | 15 doc | 775 |
| **Total** | **848** | **520** | **15** | **1383** |

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|----|----------------------|-----------------|-------------------|
| 1 | Transport skeleton: binary starts, binds loopback, answers `initialize`/`tools/list` (0 tools), shuts down | PR 1 | `go test -race ./internal/mcp/...` | `go run ./cmd/mcp-server` + `curl POST /mcp` initialize | `internal/mcp/` + `internal/buildinfo/` + main.go wiring — removable without touching auth/usecases |
| 2 | Auth + 6 tools + e2e + PRD doc fix | PR 2 | `go test -race ./internal/mcp/... ./internal/application/usecase/...` | `httptest` server + mock LLM client e2e | `auth_translator.go` + `tools_*.go` + `ports.go` + use case — removable without touching PR 1 skeleton |

## PR 1: Transport skeleton (base: `main`)

Branch: `feat/feat-mcp-transport-1` off `main`.

- [ ] **T-01** Loopback validator + package doc
  - Files: `internal/mcp/doc.go` (8), `internal/mcp/loopback.go` (45), `internal/mcp/loopback_test.go` (55)
  - Acceptance: rejects `0.0.0.0`/`localhost`/`192.168.1.5`/`::ffff:8.8.8.8`; accepts `127.0.0.1`/`127.1.2.3`/`::1`; Spanish errors per ADR-0007 §D4
  - Test: table-driven unit, no I/O
  - Commit: `feat(mcp): add loopback bind validator with Spanish error`

- [ ] **T-02** Config loader + healthz + buildinfo
  - Files: `internal/buildinfo/buildinfo.go` (8), `internal/mcp/config.go` (55), `internal/mcp/healthz.go` (22), `internal/mcp/healthz_test.go` (25), `internal/config/dotenv.go` (20)
  - Depends: T-01
  - Acceptance: `MCP_BIND`/`MCP_PORT` env precedence (flag tier reserved — none today; env > `.env` > default `127.0.0.1:3000`, per REQ-MT-013/ADR-0007); `/healthz` returns `{"status":"ok","version":"..."}`
  - Test: `t.Setenv` + `httptest` for healthz
  - Commit: `feat(mcp): add config loader, healthz endpoint, and buildinfo package`

- [ ] **T-03** Streamable HTTP transport skeleton
  - Files: `internal/mcp/server.go` (90), `internal/mcp/transport.go` (45), `internal/mcp/errors.go` (25, parse-error only), `internal/mcp/server_test.go` (55)
  - Depends: T-01, T-02
  - Acceptance: `initialize` returns `protocolVersion: "2025-11-25"` + capabilities; `tools/list` returns 0 tools; `GET /mcp` → 405; malformed JSON → `-32700`
  - Test: `httptest.NewServer` + real `mcp.NewServer` (Stateless mode)
  - Commit: `feat(mcp): integrate go-sdk v1.2.0 with NewStreamableHTTPHandler`

- [ ] **T-04** Graceful shutdown
  - Files: `internal/mcp/shutdown.go` (55), `internal/mcp/shutdown_test.go` (45)
  - Depends: T-03
  - Acceptance: SIGTERM/SIGINT → `http.Server.Shutdown(ctx 10s)` (per REQ-MT-010 — deadline must exceed the SQLite `busy_timeout=5000` to avoid force-closing an in-flight non-idempotent mutation); in-flight drains or force-closes; `ShutdownResult{Drained, ForceClosed}` logged
  - Test: send SIGTERM to test process with in-flight request
  - Commit: `feat(mcp): add graceful shutdown with 10s drain boundary`

- [ ] **T-05** Wire transport into main.go + add SDK dependency
  - Files: `cmd/mcp-server/main.go` (50 net), `go.mod`/`go.sum` (5)
  - Depends: T-01–T-04
  - Acceptance: replace `_ =` block + `os.Exit(0)` with real wiring; binary binds `127.0.0.1:3000`; `go build ./...` + `go test -race ./...` green
  - Test: full pre-flight pipeline
  - Commit: `feat(cmd): wire transport skeleton into composition root`

## PR 2: Auth + 6 tools + e2e (base: PR 1 branch)

Branch: `feat/feat-mcp-transport-2` off `feat/feat-mcp-transport-1`.

- [x] **T-06** Auth middleware wiring + JSON-RPC auth translator
  - Files: `internal/mcp/auth_translator.go` (70), `internal/mcp/auth_translator_test.go` (60), `cmd/mcp-server/main.go` (20, RBAC + authMW)
  - Depends: T-05
  - Acceptance: 401 → JSON-RPC `-32000` `"no se proporcionó X-Caller-Id"`; 403 → `-32001` `"no tienes permiso para realizar esta acción"`; `auth.FromContext(ctx)` populated inside tool handler (REQ-AM-WIRED-001–004)
  - Test: `httptest.ResponseRecorder` wrapping fake 401/403 inner; assert JSON-RPC body
  - Commit: `feat(mcp): add jsonrpcAuthTranslator wrapping AuthMiddleware`

- [x] **T-07** GetBusinessProfile use case
  - Files: `internal/application/usecase/get_business_profile.go` (25), `internal/application/usecase/get_business_profile_test.go` (35), `cmd/mcp-server/main.go` (5)
  - Depends: T-05
  - Acceptance: wraps `BusinessProfileRepo.Get(ctx)`; returns `*entity.BusinessProfile`; requires owner/admin/staff
  - Test: `go-sqlmock` unit test
  - Commit: `feat(usecase): add GetBusinessProfile use case`

- [x] **T-08** Consumer ports + error mapping
  - Files: `internal/mcp/ports.go` (40), `internal/mcp/errors.go` (40, full business-code map)
  - Depends: T-06, T-07
  - Acceptance: 6 consumer interfaces declared (no `internal/repository` import); `*domain.SemanticError` → `-32002` + Spanish message; infra → `-32603` generic
  - Test: error mapping unit tests (golden cases)
  - Commit: `feat(mcp): add consumer port interfaces and business error mapping`

- [x] **T-09** Register 6 tools
  - Files: `internal/mcp/tools_booking.go` (170), `internal/mcp/tools_profile.go` (35), `internal/mcp/tools_test.go` (85)
  - Depends: T-08
  - Acceptance: `tools/list` returns 6 tools; each tool dispatches to mock port; `auth.FromContext(ctx)` propagated; invalid args → `-32602`
  - Test: mock port structs, table-driven per tool
  - Commit: `feat(mcp): register 6 booking and profile tools against use case ports`

- [x] **T-10** Integration + e2e + guard tests
  - Files: `internal/mcp/server_integration_test.go` (80), `internal/mcp/e2e_test.go` (65), `internal/mcp/no_repo_import_test.go` (15)
  - Depends: T-09
  - Acceptance: `/mcp` happy path (`initialize` → `tools/list` → `tools/call check_availability`) with temp-file SQLite (WAL); 401/403 → JSON-RPC; `TestNoRepositoryImport` guard passes
  - Test: `httptest` + real use cases + temp-file SQLite (WAL)
  - Commit: `test(mcp): add /mcp integration tests and e2e mock client`

- [x] **T-11** PRD + ADR doc fix
  - Files: `docs/PRD.md` (§2.2, §3.1–§3.3, §5.2, §6.1–§6.2, §7, §8.1–§8.2, glosario — todas las menciones de "SSE"), `docs/architecture/0007-server-config.md`
  - Depends: T-10
  - Acceptance: "SSE" → "Streamable HTTP (MCP 2025-11-25)" in all cited sections
  - Commit: `docs(prd): update transport terminology from SSE to Streamable HTTP`

## Task Dependency Graph

```
T-01 → T-02 → T-03 → T-04 → T-05 ─────────────────────────────────┐
                                   ├→ T-06 → T-08 → T-09 → T-10 → T-11  (PR 2)
                                   └→ T-07 ─┘
```

## Implementation Order

1. **T-01**: Create `internal/mcp/` package with loopback validator — foundational security gate.
2. **T-02**: Add config loading (env precedence), healthz, and buildinfo — needed by server.
3. **T-03**: Integrate go-sdk, build server/transport, verify `initialize`/`tools/list` over httptest.
4. **T-04**: Add graceful shutdown with signal handling and drain boundary.
5. **T-05**: Wire everything into `main.go`, add SDK to `go.mod`, verify binary starts and binds.
6. **T-06**: Add auth translator + wire `AuthMiddleware` in main.go — 401/403 → JSON-RPC.
7. **T-07**: Create `GetBusinessProfileUseCase` — 6th tool prerequisite.
8. **T-08**: Declare consumer ports + complete error mapping (business → `-32002`).
9. **T-09**: Register all 6 tools with typed handlers calling use case ports.
10. **T-10**: Write integration + e2e + architecture guard tests.
11. **T-11**: Fix PRD/ADR terminology (SSE → Streamable HTTP).

## Commit Strategy

### PR 1 (5 commits, base: `main`)

1. `feat(mcp): add loopback bind validator with Spanish error`
2. `feat(mcp): add config loader, healthz endpoint, and buildinfo package`
3. `feat(mcp): integrate go-sdk v1.2.0 with NewStreamableHTTPHandler`
4. `feat(mcp): add graceful shutdown with 10s drain boundary`
5. `feat(cmd): wire transport skeleton into composition root`

### PR 2 (6 commits, base: `feat/feat-mcp-transport-1`)

1. `feat(mcp): add jsonrpcAuthTranslator wrapping AuthMiddleware`
2. `feat(usecase): add GetBusinessProfile use case`
3. `feat(mcp): add consumer port interfaces and business error mapping`
4. `feat(mcp): register 6 booking and profile tools against use case ports`
5. `test(mcp): add /mcp integration tests and e2e mock client`
6. `docs(prd): update transport terminology from SSE to Streamable HTTP`

> **Adjustment from design §9**: split design's commit 8 (ports + errors + 6 tools + tests = ~365 prod) into T-08 (ports + errors) and T-09 (6 tools) because the combined LOC exceeds the 200-LOC-per-task guideline. Added T-08 as a separate commit to keep error-mapping reviewable independently from tool registration.

## PR Strategy Summary

| PR | Branch | Base | Commits | Prod LOC | Test LOC | Total changed |
|----|--------|------|---------|----------|----------|---------------|
| PR 1 | `feat/feat-mcp-transport-1` | `main` | 5 | 428 | 180 | 608 |
| PR 2 | `feat/feat-mcp-transport-2` | `feat/feat-mcp-transport-1` | 6 | 420 | 355 | 775 |
| **Total** | | | **11** | **848** | **535** | **1383** |

Chain strategy: `feature-branch-chain` — only the tracker branch (`feat/feat-mcp-transport-2`) merges to `main` after both PRs are reviewed.

## Pre-flight Gates (per commit)

- [ ] `gofmt -l .` — empty output
- [ ] `go vet ./...` — clean
- [ ] `go build -o /dev/null ./...` — passes
- [ ] `go test -v -race ./...` — passes
- [ ] `golangci-lint run ./...` — 0 issues
- [ ] `govulncheck ./...` — 0 vulnerabilities (dependency audit; PR 1 T-05 adds the only new module, go-sdk, so the gate runs from that commit onward)
- [ ] GGA (Gentle Guardian Angel) — runs on commit, must pass

## Open Questions for Implementer

1. **Q-O1 (dotenv)**: `internal/config/dotenv.go` does NOT exist today. T-02 adds it (~20 LOC). Verify at apply whether Fase 1 shipped it; if so, remove from T-02.
2. **JSON-RPC "tool not found" code**: use SDK default `-32601` (Method not found). No custom code needed.
3. **`0.0.0.0` reject**: confirmed — explicit Spanish message per ADR-0007 §D4 (T-01).
4. **Auth translator message source**: use spec's literal Spanish strings for `-32000`/`-32001` (REQ-AM-WIRED-002/003 mandate them verbatim).
5. **golangci-lint config**: use project's existing `.golangci.yml` (or create one if missing).

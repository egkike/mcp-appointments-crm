```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:060b4225e3324611e3a7fb4347d9876e32ec4f9d69feaa77dfe688bc73a859ae
verdict: fail
blockers: 0
critical_findings: 1
requirements: 9/11
scenarios: 12/14
test_command: go test -v -race ./...
test_exit_code: 0
test_output_hash: sha256:5466d15c8c8a4222fb9d940dec72eb870ebeefcadfbee072cc5d900ac1d02814
build_command: go build -o /dev/null ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report — feat-mcp-transport, PR 1 (T-01..T-05)

**Change**: feat-mcp-transport (PR 1 slice: Transport skeleton)
**Version**: spec 2026-08-05
**Mode**: Strict TDD
**Scope**: PR 1 only — branch `feat/feat-mcp-transport-1` @ 6f7310b (base `main` @ 0d9628e). T-06..T-11 (auth translator, 6 tools, e2e, PRD doc fix) are PR 2 scope and are NOT judged here except for scope-drift control.

**Scoping note**: this is an interim PR-slice verification, not the change-final verify. Envelope totals count the 11 requirements / 14 scenarios assigned to PR 1 by `tasks.md` (T-01..T-05). Full-change spec totals for the final verify are 20 requirements / 29 scenarios (16 REQ-MT-* + 4 REQ-ARCH-INTMCP-*).

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total (PR 1: T-01..T-05) | 5 |
| Tasks complete | 5 |
| Tasks incomplete | 0 |

All five PR 1 checkboxes are marked `[x]` in `tasks.md`; PR 2 tasks (T-06..T-11) remain `[ ]` as planned.

## Build & Tests Execution

**Build**: ✅ Passed — `go build -o /dev/null ./...` exit 0 (empty output; hash `sha256:e3b0c442…b855`).

**Tests**: ✅ 228 passed / 0 failed / 0 skipped — `go test -v -race ./...` exit 0, 10/10 packages `ok`, 0 `DATA RACE` reports. Fresh uncached run (`-count=1`) reproduced: 228/0/0. Output hash `sha256:5466d15c…2814`.

**Pre-flight (read-only gates)**:
- `gofmt -l .` → empty ✅
- `go vet ./...` → clean ✅
- `golangci-lint run ./...` → 0 issues ✅
- `govulncheck ./...` (via ~/go/bin) → "No vulnerabilities found" ✅ — confirms the go-sdk v1.2.0→v1.4.1 CVE remediation is effective on the pinned tree.

**Live runtime evidence (verifier-executed binary smoke test)**: binary `go build ./cmd/mcp-server` with `MCP_DB_PATH` on a temp file:
- binds `127.0.0.1:3000` by default; `MCP_PORT=4000` → binds `127.0.0.1:4000`, healthz 200 ✅
- `GET /healthz` → HTTP 200 `{"status":"ok","version":"dev"}` ✅
- `initialize` (protocolVersion 2025-11-25) → HTTP 200, `protocolVersion:"2025-11-25"`, serverInfo, `capabilities.tools` ✅
- `tools/list` → HTTP 200, `tools: []` (0 tools — PR 1 skeleton contract) ✅
- `GET /mcp` → HTTP 405 ✅
- malformed JSON → HTTP 400 body `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}` ✅
- `tools/call nonexistent_tool` → `{"code":-32602,"message":"unknown tool \"nonexistent_tool\""}` (live REQ-MT-006 conflict evidence, see Deviations)
- `MCP_BIND=0.0.0.0` → exit 1, error contains verbatim `Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).` (wrapped in `bind address is not loopback: …`); `MCP_BIND=192.168.1.5` → non-loopback Spanish error ✅
- DB file corrupted mid-run → `/healthz` still HTTP 200 (REQ-MT-014 liveness-only scenario, verifier runtime evidence) ✅
- SIGTERM → `shutdown: drain requested` → `shutdown: drained cleanly` → `mcp server stopped cleanly drained=0 force_closed=0`, process exit code 0 ✅

**Coverage** (changed files, `-coverpkg=./internal/mcp,./internal/config,./internal/buildinfo`): **85.3%** aggregate → ⚠️ Acceptable (≥80%, <95%). See Changed File Coverage.

## Spec Compliance Matrix (PR 1 scope)

| Requirement | Scenario | Test / Evidence | Result |
|-------------|----------|------|--------|
| REQ-MT-001 Loopback bind only | Default bind succeeds | `loopback_test.go > TestValidateLoopback` + live default-bind smoke | ✅ COMPLIANT |
| REQ-MT-001 | Non-loopback rejected | `TestValidateLoopback/wildcard_rejected` + live `MCP_BIND=0.0.0.0` exit 1 verbatim Spanish msg | ✅ COMPLIANT |
| REQ-MT-002 Single POST endpoint | POST /mcp accepted | `server_test.go > TestServerInitialize`, `TestServerToolsListEmpty` + live POST 200 | ✅ COMPLIANT |
| REQ-MT-002 | GET /mcp rejected | `TestServerGetMethodNotAllowed` + live GET 405 | ✅ COMPLIANT |
| REQ-MT-003 JSON-RPC 2.0 framing | Malformed JSON | `server_test.go > TestServerMalformedJSON` (-32700 envelope, id null) + live | ✅ COMPLIANT |
| REQ-MT-004 Initialize handshake | Initialize returns capabilities | `TestServerInitialize` (2025-11-25, serverInfo, capabilities.tools) + live | ✅ COMPLIANT |
| REQ-MT-010 Graceful shutdown | SIGTERM drains | `shutdown_test.go > TestRunDrainsActiveConnectionOnSignal`, `TestRunForceClosesAfterDeadline` (deadline enforced), `TestRunCatchesSIGTERM` + live SIGTERM exit 0; `shutdownDeadline = 10s > busy_timeout 5s` in code | ✅ COMPLIANT |
| REQ-MT-013 Configuration via env vars | Custom port | `config_test.go > TestLoadConfigEnvPartialOverride` (MCP_PORT=4000) + live bind on :4000; full precedence env > .env > default covered by `TestLoadConfigEnvOverridesDotEnv`, `TestLoadConfigDotEnvTier`, `TestLoadConfigDefaults` | ✅ COMPLIANT |
| REQ-MT-014 Health check | Health check passes | `healthz_test.go > TestHealthz` + live | ✅ COMPLIANT |
| REQ-MT-014 | DB unreachable still reports liveness | Verifier live evidence (DB corrupted mid-run → healthz 200); no repo regression test | ✅ COMPLIANT (verifier runtime evidence; see W-4) |
| REQ-ARCH-INTMCP-001 Layer exists | Layer exists | `internal/mcp/` with 7 production files implementing the transport | ✅ COMPLIANT |
| REQ-ARCH-INTMCP-002 Composition root | Wiring in cmd/ | `main.go` constructs transport + mux + Run; use case injection into tools is PR 2 (T-09) | ⚠️ PARTIAL (by design, completes in PR 2) |
| REQ-ARCH-INTMCP-003 Consumer interfaces | No direct repository import | grep of `internal/mcp/` → zero `internal/repository` imports (verified); `ports.go` + guard test are T-08/T-10 (PR 2) | ⚠️ PARTIAL (invariant holds; interfaces + guard pending PR 2) |
| REQ-ARCH-INTMCP-004 Adapter conventions | Structured logging used | `*slog.Logger` in server/shutdown/main; `errors.Is(err, http.ErrServerClosed)`; ctx propagation; defers; Spanish loopback errors; `%w` wrapping | ✅ COMPLIANT (PR 1 surface; SemanticError mapping is PR 2) |

**Compliance summary**: 12/14 PR 1 scenarios compliant (2 partial by design — their remaining work is explicitly scheduled in PR 2 tasks T-06..T-10).

PR 2-scoped requirements (REQ-MT-005/006/007/008/009/011/012/015/016) are intentionally not implemented in this slice — confirmed by scope-drift check below. REQ-MT-011 (structured request logging) has no task in either PR's task list — see W-5.

## Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| REQ-MT-001 | ✅ Implemented | `ValidateLoopback` exact ADR-0007 §D4 messages; called in `main.go` before DB open and before any listener |
| REQ-MT-002 | ✅ Implemented | SDK `Stateless:true` → GET 405 (verified live) |
| REQ-MT-003 | ✅ Implemented | `jsonParseGuard` buffers once, 1 MiB cap → 413, invalid JSON → -32700 envelope; body restored for inner handler |
| REQ-MT-004 | ✅ Implemented | SDK v1.4.1 `supportedProtocolVersions` contains 2025-11-25; `negotiatedVersion` echoes client-requested 2025-11-25 |
| REQ-MT-010 | ✅ Implemented | `Run`/`run`: signal.Notify(SIGTERM,SIGINT), `Shutdown(ctx 10s)`, second signal force-close, ConnState drained/force-closed accounting, serve-failure propagation |
| REQ-MT-013 | ✅ Implemented | `LoadConfig` env > .env > default; in-house `LoadDotEnv` (ADR-0007 §D5, Q-O1 resolved: file added) |
| REQ-MT-014 | ✅ Implemented | `Healthz(version)` — no DB dependency in signature (liveness-only by construction) |
| REQ-ARCH-INTMCP-001/004 | ✅ Implemented | layer + conventions (slog, ctx, defer, errors.Is, %w) |

## Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| go-sdk pinned v1.2.0 (design §4/§10) | ⚠️ Deviation | Bumped to v1.4.1 — 4 CVEs in v1.2.0 (GO-2026-5771/4773/4770/4569); `govulncheck` is a hard pre-commit gate (AGENTS.md). Documented in apply-progress + commit ba86acd. Verified: all tests pass on v1.4.1; govulncheck clean; see Deviation assessment |
| `LoadConfig() (Config, error)` vs design's no-error signature | ⚠️ Deviation | Fail-fast on unreadable .env; missing file silent. Documented (apply deviation #1). Tested both paths |
| `run`/`Run` own listener + Serve; `(ShutdownResult, error)` return | ⚠️ Deviation | Design §3 had `mcp.Run(httpSrv)`; actual `Run(ctx, srv, logger)` — serve failure propagates so systemd sees non-zero exit (deviations #2/#3). Improvement over design |
| Stateless:true + JSONResponse:true | ✅ Yes | `transport.go`; GET 405 verified |
| 1 MiB body bound at outermost layer | ✅ Yes | `jsonParseGuard` (SDK v1.4.1 still has no `MaxRequestBodyBytes` — verifier re-checked streamable.go); 413 branch untested (W-2) |
| http.Server timeouts (5s/30s/30s/60s) | ✅ Yes | main.go matches design §3 |
| main() → run() error pattern; os.Exit only in main | ✅ Yes | gocritic exitAfterDefer resolved (deviation #7) |
| Second signal force-closes during drain | ✅ Yes (deviation #5) | Implemented; no covering test (W-3) |
| PR 1 = skeleton only, no auth/tools | ✅ Yes | Scope-drift check below |

### Deviation assessment

1. **go-sdk v1.2.0 → v1.4.1**: **consistent**. Security-driven, documented in apply-progress and commit ba86acd body, forced by the mandatory `govulncheck` pre-commit gate. Verifier re-verified the v1.4.1 API surface: (a) `StreamableHTTPOptions` still lacks `MaxRequestBodyBytes` → the custom `jsonParseGuard` remains necessary and correct; (b) `supportedProtocolVersions` includes 2025-11-25 → REQ-MT-004 satisfied; (c) v1.4.1 adds default-on DNS-rebinding protection (`DisableLocalhostProtection`) and `CrossOriginProtection` — compatible security hardening for the loopback design (Hermes uses a localhost Host header). One stale comment: `errors.go` says "Verified against go-sdk v1.2.0" while go.mod pins v1.4.1 (S-1).
2. **REQ-MT-006 unknown-tool code (-32602 vs -32601)**: **inconsistent with spec text as of PR 1** (live evidence: `tools/call nonexistent_tool` → `-32602 "unknown tool \"nonexistent_tool\""`; SDK v1.4.1 `mcp/server.go:738` hardcodes `CodeInvalidParams`). Spec scenario mandates `-32601`. The conflict is real, persists in v1.4.1, is documented in apply-progress, and is explicitly deferred to PR 2 ("resolve REQ-MT-006 unknown-tool code"). Not a PR 1 gap (0 tools registered; dispatch is PR 2), but it MUST be resolved in PR 2 (translate to -32601 or amend the spec) — if PR 2 ships without resolution it becomes a spec violation.

## Scope Drift Check (T-06..T-11 must NOT be in this branch)

✅ **No scope drift.** `internal/mcp/` contains exactly the PR 1 file set (doc, loopback, config, healthz, server, transport, errors, shutdown + 5 test files). No `auth_translator.go`, no `tools_*.go`, no `ports.go`, no integration/e2e/guard tests. `main.go` wires no auth middleware; use cases/repos remain explicit `_ =` placeholders with comments deferring to PR 2. Diff vs `main` (18 files, +1250/−16) matches the PR 1 plan.

## TDD Compliance (Strict Mode)

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ❌ | No "TDD Cycle Evidence" table in apply-progress.md (Commits, Deviations, SDK facts, Test summary sections exist; no per-task RED/GREEN/TRIANGULATE/SAFETY NET/REFACTOR evidence) |
| All tasks have tests | ✅ | 5/5 tasks have test files (loopback_test, config_test, healthz_test, server_test, shutdown_test) |
| RED confirmed (tests exist) | ✅ | 5/5 test files verified present |
| GREEN confirmed (tests pass) | ✅ | 5/5 test files pass under `-race` on fresh execution |
| Triangulation adequate | ✅ | loopback 7 cases, config 6, server 4, shutdown 5; healthz single-case (behavior is single-scenario for the repo test; second scenario verifier-covered) |
| Safety Net for modified files | ➖ | All PR 1 files are new; no modified pre-existing files (main.go extended in-place by design) |

**TDD Compliance**: 5/6 checks passed. The missing TDD Cycle Evidence table is CRITICAL per strict-tdd-verify.md: the apply phase did not report per-task cycle evidence, so RED-phase adherence (test written and failing first) cannot be proven from artifacts. The verifiable substance (tests exist, pass, triangulated, high assertion quality) is nonetheless strong — remediation is documentation, not code.

## Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 13 | 2 (`loopback_test.go`, `config_test.go`) | go test |
| Integration | 10 | 3 (`healthz_test.go`, `server_test.go`, `shutdown_test.go` — httptest, real http.Server, real signals) | go test, net/http/httptest |
| E2E | 0 | 0 | planned T-10 (PR 2) |
| **Total** | **23** | **5** | |

## Changed File Coverage

| File | Line % | Branch % | Uncovered Lines | Rating |
|------|--------|----------|-----------------|--------|
| `internal/mcp/loopback.go` | 100% | — | — | ✅ Excellent |
| `internal/mcp/healthz.go` | 100% | — | — | ✅ Excellent |
| `internal/mcp/server.go` | 100% | — | — | ✅ Excellent |
| `internal/mcp/transport.go` | 100% | — | — | ✅ Excellent |
| `internal/mcp/doc.go` | 100% | — | — | ✅ Excellent |
| `internal/buildinfo/buildinfo.go` | 100% | — | — | ✅ Excellent |
| `internal/config/dotenv.go` | 91.3% | — | scanner-error branch | ✅ Excellent |
| `internal/mcp/errors.go` | ~87% | — | L36-39 body-read-error; L40-43 **413 oversized-body branch** | ⚠️ Acceptable |
| `internal/mcp/shutdown.go` | ~88% | — | L44-46 nil-logger; L49-51 listen-error; L79-81 prevConnState; L111-113 ErrServerClosed-before-signal; **L140-144 second-signal force-close** | ⚠️ Acceptable |
| `internal/mcp/config.go` | ~80% | — | L36-44 LoadConfig glue (home-dir resolution; live-verified via binary); L68 all-empty firstNonEmpty | ⚠️ Acceptable |
| `cmd/mcp-server/main.go` | n/a | — | composition root — verified via live binary smoke instead of unit tests | ➖ By convention |

**Average changed file coverage (testable files)**: 85.3% — acceptable; two behavior-bearing uncovered branches are flagged (W-2, W-3).

## Assertion Quality

**Assertion quality**: ✅ All assertions verify real behavior. 0 CRITICAL, 0 WARNING.

Audit of all 5 test files: no tautologies; no type-only assertions standing alone; no ghost loops; no smoke-only tests; no implementation-detail coupling (assertions target public API values: error substrings, config field values, HTTP status, JSON-RPC envelope fields, `ShutdownResult` counters); zero mocks (real servers/handlers/signals). `TestServerToolsListEmpty` correctly distinguishes `nil` vs empty array. `TestRunCatchesSIGTERM` self-kills with `syscall.Kill(Getpid(), SIGTERM)` behind a safety channel — real signal-path wiring.

## Quality Metrics

**Linter (golangci-lint)**: ✅ 0 issues
**Type Checker (go vet)**: ✅ clean
**gofmt**: ✅ clean
**govulncheck**: ✅ No vulnerabilities found

## Issues Found

**CRITICAL**:
- **C-1 (process evidence)**: apply-progress.md contains no TDD Cycle Evidence table. Strict TDD mode was active during apply, and strict-tdd-verify.md requires per-task RED/GREEN/TRIANGULATE/SAFETY NET/REFACTOR evidence. RED-phase adherence for T-01..T-05 is unprovable from artifacts. Remediation: apply phase documents the per-task cycle evidence (or the orchestrator explicitly accepts the gap); no code change required.

**WARNING**:
- **W-1**: REQ-MT-006 conflict unresolved: live behavior `-32602` (SDK v1.4.1 hardcodes `CodeInvalidParams` for unknown tools) vs spec scenario `-32601`. Correctly documented and deferred to PR 2; must be resolved (error-code translation or spec amendment) before PR 2 merges, else it is a spec violation.
- **W-2**: the 1 MiB body-bound 413 branch (`errors.go` L40-43) has no covering test — a security-relevant defense-in-depth control (design §4 body bound).
- **W-3**: the second-signal force-close path (`shutdown.go` L140-144, documented apply deviation #5 / GGA S-4) has no covering test.
- **W-4**: REQ-MT-014 "DB unreachable still reports liveness" scenario has no repo regression test (verifier confirmed the behavior live; handler signature makes DB access structurally impossible). Recommend adding a test in PR 2.
- **W-5**: REQ-MT-011 (structured request logging: method/path/status/duration_ms/caller_role) has no task in either PR 1 or PR 2 in tasks.md — planning gap. Only startup/shutdown/session logs exist today. Must be assigned (PR 2 or a follow-up) before the change-final verify, or the requirement will surface as UNTESTED/UNIMPLEMENTED at archive time.

**SUGGESTION**:
- **S-1**: `errors.go` comment says "Verified against go-sdk v1.2.0" while go.mod pins v1.4.1 — update the comment to v1.4.1 (the technical claim still holds; verifier re-confirmed v1.4.1 lacks `MaxRequestBodyBytes`).
- **S-2**: `internal/mcp/config.go` `LoadConfig` glue (home-dir path) is unit-uncovered (0%) though exercised by the live binary; consider a thin test or accept as composition glue.
- **S-3**: PR 2 must re-verify the go-sdk v1.4.x API surface notes recorded in apply-progress (MaxRequestBodyBytes absence confirmed by this verification; protocol constants confirmed).

## Verdict

**FAIL** (process-evidence failure; zero product/spec defects found in PR 1 scope)

All five PR 1 tasks are complete with every acceptance criterion verified by passing runtime tests plus independent live binary execution; all static gates are green; no scope drift. The single CRITICAL is the missing TDD Cycle Evidence table required by Strict TDD mode — a documentation remediation in the apply phase, not a code defect. Once the evidence table is supplied (or the gap explicitly accepted by the orchestrator), PR 1 is admission-ready and PR 2 (base: this branch) may start.

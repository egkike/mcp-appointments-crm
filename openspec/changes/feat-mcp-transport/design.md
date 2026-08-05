# Design: feat-mcp-transport

> **Change**: feat-mcp-transport
> **Phase**: Fase 2 — mcp-server-core (PRD §7)
> **Status**: Designed
> **Date**: 2026-08-05
> **Inputs**: proposal.md (Q1–Q3 resolved), specs/{mcp-transport,auth-middleware,architecture}/spec.md, Engram obs #663/#664/#665, ADRs 0005/0007/0009/0013
> **SDK verified**: Context7 `/modelcontextprotocol/go-sdk` v1.2.0 (2026-08-05)

## 1. Architecture overview

```
┌────────────┐    HTTP/Streamable (loopback)    ┌─────────────────────────────────────────────┐
│  Hermes    │ ────────── 127.0.0.1:3000 ─────── │  cmd/mcp-server/main.go  (composition root)  │
│  (LLM client)                                 │   ├─ loopback.Validate(MCP_BIND)             │
└────────────┘                                  │   ├─ http.Server → mux{/mcp, /healthz}       │
                                                 │   ├─ SIGTERM/SIGINT → Shutdown(ctx 10s)
                                                 │   └─ defer database.Close()                  │
                                                 │              │                                │
   POST /mcp  ──────────────────────────────────►│  jsonrpcAuthTranslator(authMW.Wrap(mcpHandler))
   GET  /mcp  ──► 405 (SDK Stateless mode)       │              │                                │
   GET  /healthz ──► 200 {status,version}        │              ▼                                │
                                                 │  internal/mcp/  (NEW adapter layer)          │
                                                 │   ├─ Server  → mcp.NewServer + AddTool ×6   │
                                                 │   └─ mcp.NewStreamableHTTPHandler (Stateless)│
                                                 │              │                                │
                                                 │              ▼  consumes use case interfaces   │
                                                 │  internal/application/usecase/ (ports)      │
                                                 │              │                                │
                                                 │              ▼                                │
                                                 │  internal/repository/ ─► SQLite (WAL)        │
                                                 └─────────────────────────────────────────────┘
```

`internal/mcp/` is the Hexagonal "adapter" mandated by ADR-0013 (the diagram's missing right-hand adapter). The **`github.com/modelcontextprotocol/go-sdk`** lives INSIDE this adapter — it is a transport implementation detail, never imported by `internal/domain/` or `internal/application/usecase/`. The domain use cases have no knowledge of JSON-RPC, HTTP, or the SDK.

Composition order on the `/mcp` route (outer→inner):
`jsonrpcAuthTranslator` → `auth.AuthMiddleware.Wrap` → `mcp.StreamableHTTPHandler` → tool handler → use case.

Loopback validation is NOT a per-request wrapper; ADR-0007 mandates it run **before `ListenAndServe`** (startup fail-fast). Per-request wrapping would contradict the ADR and waste cycles.

## 2. Package structure: `internal/mcp/`

| File | Responsibility | Est. LOC |
|------|----------------|----------|
| `server.go` | `Server` struct + `NewServer(Config)`; owns `*mcp.Server`, registers handler on a `*http.ServeMux`. | ~120 |
| `config.go` | `Config{Bind,Port,Version,Logger}`; `LoadConfig()` reads `MCP_BIND`/`MCP_PORT` per ADR-0007 precedence (env > `.env`¹ > defaults `127.0.0.1:3000`). | ~70 |
| `loopback.go` | `ValidateLoopback(bind string) error`; `net.ParseIP` + `IsLoopback`; rejects `0.0.0.0`, hostnames, non-loopback. | ~50 |
| `transport.go` | `streamableHandler(srv)` wraps `mcp.NewStreamableHTTPHandler(func(*http.Request)*mcp.Server, &StreamableHTTPOptions{Stateless:true, JSONResponse:true})`. | ~60 |
| `auth_translator.go` | `jsonrpcAuthTranslator` wraps `http.Handler`; intercepts 401/403 from `AuthMiddleware` (captured via `statusRecorder`) and re-emits JSON-RPC `-32000`/`-32001` per REQ-AM-WIRED-002/003. | ~90 |
| `errors.go` | `mapError(err) (code int, msg string)`; `errors.As` for `*domain.SemanticError` → Spanish message + code; default `-32603`. | ~80 |
| `tools_booking.go` | Registers `check_availability`, `create_booking`, `get_booking`, `cancel_booking`, `reschedule_booking`; typed `AddTool[In,Out]` handlers calling use cases. | ~220 |
| `tools_profile.go` | Registers `get_business_profile`; calls `BusinessProfileGetter.Execute(ctx)`. | ~50 |
| `ports.go` | Consumer-side interfaces (per data-access C5) the handlers depend on. | ~55 |
| `healthz.go` | `healthzHandler(version)` → `200 {"status":"ok","version":"<x.y.z>"}`. | ~30 |
| `shutdown.go` | `Run(ctx, srv)`; `signal.Notify(SIGTERM,SIGINT)`; `http.Server.Shutdown(ctx 10s)`; logs drained/force-closed counts. | ~70 |
| `doc.go` | Package doc comment. | ~10 |
| `*_test.go` (×6) | Unit + integration + e2e (see §8). | ~500 |

¹ `.env` parsing is implemented in `internal/config/dotenv.go` (in-house per ADR-0007 §D5). If that package does not yet exist at apply time, this change adds it (~20 LOC) as a PR 1 work unit. **Open question Q-O1** tracks whether it already ships with Fase 1.

**Import graph** (enforced by `go vet` + a grep guard test):
- `internal/mcp/` imports: `net/http`, `log/slog`, `context`, `encoding/json`, `errors/fmt`, `os/signal`, `github.com/modelcontextprotocol/go-sdk/mcp`, `internal/application/dto`, `internal/application/usecase` (interfaces only, via `ports.go`), `internal/auth` (Caller extraction), `internal/domain` (SemanticError).
- `internal/mcp/` does **NOT** import `internal/repository`. A compile-time guard test (`TestNoRepositoryImport`) greps the package source for the forbidden import path and fails otherwise (REQ-MT-012, REQ-ARCH-INTMCP-003).

**Consumer interfaces** (`ports.go`) — declared in the consumer package per C5; concrete use cases satisfy them structurally (same pattern as `internal/application/usecase/validator.go`):

```go
// ports.go — interfaces the transport consumes. The composition root injects
// concrete *usecase.*UseCase values, which satisfy these structurally.
type CheckAvailabilityPort interface{ Execute(context.Context, dto.CheckAvailabilityInput) (*dto.CheckAvailabilityResult, error) }
type CreateBookingPort      interface{ Execute(context.Context, dto.CreateBookingInput)    (*dto.CreateBookingResult,    error) }
type GetBookingPort          interface{ Execute(context.Context, dto.GetBookingInput)       (*dto.GetBookingResult,       error) }
type CancelBookingPort       interface{ Execute(context.Context, dto.CancelBookingInput)    (*dto.CancelBookingResult,    error) }
type RescheduleBookingPort   interface{ Execute(context.Context, dto.RescheduleBookingInput)(*dto.RescheduleBookingResult,error) }
type BusinessProfilePort     interface{ Execute(context.Context)                            (*entity.BusinessProfile,     error) } // NEW use case
```

> `BusinessProfilePort.Execute(ctx)` requires a NEW trivial use case `GetBusinessProfileUseCase` wrapping `repository.BusinessProfileRepo.Get` (Q3). It lives in `internal/application/usecase/get_business_profile.go` (~25 LOC) and is wired in `main.go`. The repo interface it consumes already exists in `internal/domain/repository/business_profile.go`.

**Public types**: `Server`, `Config`, `LoopbackError`, `ShutdownResult{Drained,ForceClosed int}`.

## 3. Composition root: `cmd/mcp-server/main.go` (extended)

The existing 146-LOC file currently wires 9 repos + 5 use cases into `_ =` and exits. This change **replaces the `_ =` block + `os.Exit(0)`** (lines 121–146) with continued wiring down to a running HTTP server. The DB-open + `defer database.Close()` (lines 52–66) stay **untouched** and now run at real shutdown, not at instant exit.

Additions in wiring order (continues from existing use-case block):

```go
// (existing) checkAvailabilityUC ... rescheduleBookingUC remain.

// NEW 6th use case (Q3): get_business_profile.
getBusinessProfileUC := usecase.NewGetBusinessProfileUseCase(bizProfRepo)

// Auth: resolver + middleware (existing types in internal/auth).
resolver := auth.NewCallerResolver(database.Conn)
rbac := auth.ToolRBAC{
    "create_booking":      {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
    "cancel_booking":      {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
    "reschedule_booking":  {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
    "get_booking":         {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
    "get_business_profile":{auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
    // check_availability: any authenticated caller (no RBAC entry).
}
authMW := auth.NewAuthMiddleware(resolver, rbac, logger)

// MCP server (new adapter).
cfg := mcp.LoadConfig() // MCP_BIND / MCP_PORT / version via -ldflags
if err := mcp.ValidateLoopback(cfg.Bind); err != nil { logger.Error(err.Error()); os.Exit(1) }

srv := mcp.NewServer(mcp.Config{
    Version: cfg.Version, Logger: logger,
    CheckAvailability:   checkAvailabilityUC,
    CreateBooking:       createBookingUC,
    GetBooking:         getBookingUC,
    CancelBooking:      cancelBookingUC,
    RescheduleBooking:  rescheduleBookingUC,
    GetBusinessProfile: getBusinessProfileUC,
})

mux := http.NewServeMux()
mux.Handle("/mcp", srv.Handler(authMW)) // translator(authMW.Wrap(streamableHandler))
mux.Handle("/healthz", mcp.Healthz(cfg.Version))

httpSrv := &http.Server{Addr: net.JoinHostPort(cfg.Bind, cfg.Port), Handler: mux}
logger.Info("mcp server starting", "bind", cfg.Bind, "port", cfg.Port, "version", cfg.Version)

shutdown := mcp.Run(httpSrv) // blocks until SIGTERM/SIGINT, returns ShutdownResult
logger.Info("mcp server stopped", "drained", shutdown.Drained, "force_closed", shutdown.ForceClosed)
// defer database.Close() runs here.
```

**Signal handling** (exact pattern): `sigCh := make(chan os.Signal, 1); signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT); <-sigCh; ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second); defer cancel(); httpSrv.Shutdown(ctx)`. `Shutdown` returns in ≤10s; `ShutdownResult` counts how many in-flight requests finished vs. were force-closed at the 10s boundary (tracked via a `sync.WaitGroup` of accepted connections + a `done` channel).

**Shutdown deadline vs SQLite busy_timeout** (reviews R4-002): the shutdown deadline (10s) is deliberately **longer than the project-mandated `_busy_timeout=5000`** (AGENTS.md). Rationale: a non-idempotent in-flight mutation (`create_booking`) blocked on the write lock can wait up to 5s for the lock before committing; if the shutdown deadline were 5s it would force-close at the exact lock-acquisition boundary, silently drop the JSON-RPC response, and drive a client retry that creates a **duplicate booking**. At 10s the in-flight mutation has margin to acquire the lock and commit with its response before the deadline, so force-close at the boundary is reserved for genuinely stuck requests. The 5s figure from the earlier spec draft is revised to 10s in REQ-MT-010; no idempotency-key/dedup is introduced in this change (out of scope), but the deadline headroom is the documented mitigation. This is tracked as follow-up FEAT-1 for a future idempotency-key on booking mutation tools.

**Loopback validation** (exact): `ip := net.ParseIP(bind); if ip == nil { return hostnameError }; if !ip.IsLoopback() { return nonLoopbackError }`. Exact Spanish messages from ADR-0007 §D4:
- `0.0.0.0` → `Error: MCP_BIND=0.0.0.0 expone el server en TODAS las interfaces. Use solo direcciones loopback (127.0.0.0/8 o ::1).`
- non-loopback IPv4/IPv6 → `Error: MCP_BIND=<v> no es una dirección loopback. Use 127.0.0.1 (IPv4) o ::1 (IPv6).`
- hostname literal → `Error: MCP_BIND=<v> es un hostname, no una IP. Use la IP literal (127.0.0.1 o ::1).`

## 4. MCP SDK integration

**Module addition**: `github.com/modelcontextprotocol/go-sdk v1.2.0` (pinned; see §10 for transitive audit). Requires Go 1.24+; project is 1.26.4 ✅.

**Public API used** (verified via Context7 2026-08-05):
- `mcp.NewServer(impl *mcp.Implementation, opts *mcp.ServerOptions) *mcp.Server`
- `mcp.AddTool[In, Out any](s *mcp.Server, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out])` — typed handler `func(ctx, *CallToolRequest, In) (*CallToolResult, Out, error)`
- `mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server, *mcp.StreamableHTTPOptions) *mcp.StreamableHTTPHandler` → implements `http.Handler`
- `mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: ...}`
- `mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{ListChanged: false}}}`
- Tool input/output schemas inferred from Go structs via `jsonschema:"..."` tags (no hand-written JSON schemas).

**Stateless mode decision** (resolves R1/R4 + spec REQ-MT-002): `Stateless: true` makes `GET /mcp` return `405 Method Not Allowed` (satisfies REQ-MT-002 directly) and uses ephemeral per-request sessions — no `Mcp-Session-Id` header, no goroutine-leak from abandoned sessions (SDK issue #499). `initialize` still negotiates `protocolVersion: "2025-11-25"` (the highest the SDK advertises, satisfying REQ-MT-004). **Resolved question Q-A1**: the combination "2025-11-25 advertised + stateless transport" is intentional — 2025-11-25 is the maximum SDK-supported protocol version and is orthogonal to session state. This also future-proofs for the 2026-07-28 downward spec revision (SEP-2567) without a code change. Hermes v0.20 (obs #664) speaks Streamable HTTP and works stateless by default.

**`X-Caller-Id` extraction**: `auth.AuthMiddleware` runs **outside** the SDK handler and injects the resolved `Caller` into `r.Context()` via `auth.WithCaller`. The SDK derives tool-handler `ctx` from the request context, so handlers call `auth.FromContext(ctx)` (REQ-MT-007). Verified composition model — the SDK middleware example shows `r *http.Request` reaches the server factory and context propagates to tool handlers. RED test asserts `auth.FromContext(ctx)` is populated inside a registered tool.

**Tool registration** (one paragraph each — all use typed `mcp.AddTool` with struct-tag JSON schemas):

- **`check_availability`**: input `{professional_id?, service_id?, start_datetime, end_datetime?}` → `CheckAvailabilityPort.Execute`. Roles: any authenticated. Output `{available: bool, message?: string}`. Handler builds `dto.CheckAvailabilityInput{Caller: ctxCaller, ...}`, returns the result.
- **`create_booking`**: input `{client_id, service_id, professional_id, start_datetime, notes?}` → `CreateBookingPort.Execute`. Roles: owner/admin/staff. The RBAC entry in `main.go` enforces coarse-grained rejection; `auth.RequireClientMatch` inside the use case enforces fine-grained (staff calendar / client self). Output `{booking_id, start_datetime, end_datetime}`.
- **`get_booking`**: input `{booking_id}` → `GetBookingPort.Execute`. `auth.AuthorizeBookingAccess` (inside use case) enforces cross-tenant isolation. Output full `BookingView`.
- **`cancel_booking`**: input `{booking_id, reason}` → `CancelBookingPort.Execute`. Roles: owner/admin/staff. Output `{status: "cancelled"}`.
- **`reschedule_booking`**: input `{booking_id, new_start_datetime}` → `RescheduleBookingPort.Execute`. Roles: owner/admin/staff. Output `{booking_id, start_datetime, end_datetime}`.
- **`get_business_profile`** (NEW): input `{}` → `BusinessProfilePort.Execute(ctx)`. Roles: owner/admin/staff. Output the full `entity.BusinessProfile` serialised via the SDK's output-schema inference. (Lazy-init in the repo guarantees a row always exists.)

Each handler follows one shape (sketch for `create_booking`):

```go
mcp.AddTool(server, &mcp.Tool{Name:"create_booking", Description:"Crea una reserva"}, func(
    ctx context.Context, req *mcp.CallToolRequest, in CreateBookingIn,
) (*mcp.CallToolResult, CreateBookingOut, error) {
    caller, _ := auth.FromContext(ctx) // REQ-MT-007; middleware guarantees presence
    res, err := deps.CreateBooking.Execute(ctx, dto.CreateBookingInput{
        Caller: caller, ClientID: in.ClientID, ServiceID: in.ServiceID,
        ProfessionalID: in.ProfessionalID, StartTime: in.StartDatetime, Notes: in.Notes,
    })
    if err != nil { return nil, CreateBookingOut{}, toMCPError(err) }
    return nil, CreateBookingOut{BookingID: res.BookingID, /* … times … */}, nil
})
```

**Business errors → JSON-RPC**: `toMCPError(err)` in `errors.go` does `errors.As(err, &se *domain.SemanticError)` → returns `mcp.JSONRPCError{Code:-32002, Message: se.Message}`. Non-semantic wrapped errors → log Spanish context server-side (`fmt.Errorf("crear reserva: %w", err)` already wrapped by the use case), return generic `{Code:-32603, Message:"error interno del servidor"}` to the client (no stack/SQL leak, per AGENTS.md).

**Auth errors → JSON-RPC** (REQ-AM-WIRED-002/003): the existing `auth.AuthMiddleware` writes **plaintext** `http.Error(...)` on 401/403 and short-circuits — the SDK handler never runs, so the SDK cannot frame a JSON-RPC envelope. Because `internal/auth/` must stay transport-agnostic (it can't know about JSON-RPC), the JSON-RPC framing lives in `internal/mcp/auth_translator.go`: a `jsonrpcAuthTranslator` wraps `authMW.Wrap(sdkHandler)` with a `statusRecorder` `http.ResponseWriter` that buffers the auth layer's 401/403 plaintext response, then **re-emits** (preserving the request `id`, see below) `{"jsonrpc":"2.0","error":{"code":-32000,"message":"no se proporcionó X-Caller-Id"},"id":"<request id>"}` (for 401) / `code:-32001` with `"no tienes permiso para realizar esta acción"` (for 403). The Spanish message the middleware wrote is reused as the JSON-RPC `message` where the spec mandates it literally; otherwise the translator uses the spec's literal Spanish string. The `id` is **preserved, never forced to `null`**: `jsonrpcAuthTranslator` is the OUTERMOST handler (composition `translator(authMW.Wrap(sdkHandler))`), so it reads and buffers `r.Body` first and parses the JSON-RPC `id` (`encoding/json` into a `json.RawMessage`) before the inner middleware runs; the buffered body is re-sent downstream via `io.NopCloser(bytes.NewReader(body))`. On 401/403 it re-emits the error with the **same `id`** the request carried (JSON-RPC 2.0 §5.1 — the response `id` MUST equal the request `id`). `id` falls back to `null` only when the request body is unparseable as JSON-RPC (the request itself could not be parsed). This preserves response/request correlation and retry safety for concurrent clients. This keeps `internal/auth/` untouched while satisfying REQ-AM-WIRED-001–004.

## 5. Configuration

**Env vars** (REQ-MT-013): `MCP_BIND` (default `127.0.0.1`), `MCP_PORT` (default `3000`). Precedence per ADR-0007: explicit flag (none today — no CLI flags) > env var > `~/.config/mcp-appointments-crm/.env` > default. `.env` parser is the in-house 20-LOC parser (ADR-0007 §D5) in `internal/config/dotenv.go`.

**Build version**: `internal/buildinfo` exposes `Version string` set via `-ldflags "-X .../internal/buildinfo.Version=$(git describe)"` in the build command (Makefile / `scripts/`). `LoadConfig()` reads `buildinfo.Version`; fallback `"dev"` if unset. `/healthz` returns it (REQ-MT-014).

**Health check**: `GET /healthz` is a **separate `http.Handler`** on the mux, NOT an SDK resource (decision: simpler, no session semantics needed, faster). Returns `200` `{"status":"ok","version":"<x.y.z>"}`. Unauthenticated by design (loopback-only, no PII).

## 6. Observability

`slog` is the project default (ADR-0013, existing main.go D2). `slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))` is called once at the top of `main()` (composition root). All `internal/mcp/` logging goes through a `*slog.Logger` injected via `Config` (never the package-global `slog.Default()` directly — testability).

**Log shapes** (REQ-MT-011):
- Request log (per request via a `loggingMiddleware` around the mux, or via the SDK method-handler middleware pattern from the SDK `examples/server/middleware`): `{method, path, status, duration_ms, caller_role}`. `caller_role` only — **never** `caller.ID`, `X-Caller-Id` value, client names, or booking notes (PII policy, AGENTS.md).
- Error log: `{request_id, error_code, message}` where `request_id` is a UUID generated per request (no correlation to caller PII).
- Startup log: `{bind, port, version, sdk_protocol_version, tools_registered: 6}`.
- Shutdown log: `{drained, force_closed}` from `ShutdownResult`.

**Auth audit**: the existing `auth.AuthMiddleware` already emits an audit log for admin/owner callers (`internal/auth/middleware.go:79–84`, hashed `caller_hash` — SHA-256 prefix, no PII). This change **reuses** that pattern unchanged; no separate `internal/auth/audit.go` exists today and none is needed. The audit log fires from inside `AuthMiddleware.Wrap`, which still runs (it is the inner handler the translator wraps). Q-AM-1 (open) tracks whether the audit key should be a dedicated `slog` sub-logger for filtering in journald — deferred, current "privileged access" line is sufficient.

## 7. Error handling

**`*domain.SemanticError` propagation**: use cases return `*domain.SemanticError` for business-rule failures (overlap, not-working-day, past, not-found, forbidden) or `fmt.Errorf("...: %w", err)` for infra failures. `internal/mcp/errors.go` `toMCPError`:
1. `errors.As(err, &se)` → `JSON-RPC code = mapBusinessCode(se.Code)`, `message = se.Message` (Spanish, verbatim from the domain).
2. Else `errors.Is(err, context.DeadlineExceeded)` → `-32001`-ish timeout (out of scope this phase; default `-32603`).
3. Else → `-32603` internal; log full wrapped chain server-side with Spanish context already in the chain.

**JSON-RPC code map** (reserved `-32000..-32099` server-error band + protocol standards):

| Source | JSON-RPC code | Message | Notes |
|--------|--------------|---------|-------|
| Malformed JSON | `-32700` | `Parse error` | SDK default (REQ-MT-003) |
| Unknown tool / method | `-32601` | `Method not found` | SDK default (REQ-MT-006) |
| HTTP 401 (no/invalid `X-Caller-Id`) | `-32000` | `no se proporcionó X-Caller-Id` (or resolver Spanish msg) | REQ-AM-WIRED-002; via translator |
| HTTP 403 (insufficient role) | `-32001` | `no tienes permiso para realizar esta acción` | REQ-AM-WIRED-003; via translator |
| `domain.SemanticError` (business) | `-32002` | `se.Message` (Spanish) | REQ-MT-009/016 |
| Infra/5xx | `-32603` | `error interno del servidor` | logged Spanish, generic to client |
| Invalid tool args (schema) | `-32602` | SDK-invalid-args Spanish | SDK-driven via struct tags |

**Spanish message policy**: every business error surfaced to the LLM is a Spanish `*domain.SemanticError.Message` already authored in the domain layer (e.g. overlap message from `create_booking`). Technical/infra errors are logged with the Spanish context wrapped by the use case (`"crear reserva: %w"`) but the LLM receives the generic `error interno del servidor` — never stack traces, file paths, or raw SQL (AGENTS.md).

**Sentinel→code mapping** in `errors.go` (used by `mapBusinessCode`):
`ErrCodeBookingOverlap`, `ErrCodeProfessionalNotWorking`, `ErrCodeSlotInPast`, `ErrCodeNotFound`, `ErrCodeForbidden`, `ErrCodeInvalidInput`, `ErrCodeBusinessClosed`, `ErrCodeSlotOutOfHours`, `ErrCodeServiceNotActive`, `ErrCodeProfessionalNotActive`, `ErrCodeConflict` → all `-32002` (the machine-readable distinction already lives in `SemanticError.Code` for any future LLM-smart routing; the JSON-RPC code stays uniform this phase).

## 8. Testing strategy

| Layer | What | Approach | Target |
|-------|------|----------|--------|
| Unit | `loopback.go` — reject `0.0.0.0`, `localhost`, `192.168.1.5`, `::ffff:8.8.8.8`; accept `127.0.0.1`, `127.1.2.3`, `::1` | table-driven, no I/O | 95%+ |
| Unit | `config.go` — env precedence | `t.Setenv`, temp `.env` | 90%+ |
| Unit | `errors.go` — semantic/infra/auth mapping | `errors.As` golden cases | 90%+ |
| Unit | `auth_translator.go` — 401/403 → JSON-RPC translation | `httptest.ResponseRecorder` wrapping a fake 401/403 inner handler; assert JSON-RPC body + code + Spanish msg | 90%+ |
| Unit | tool handlers (×6) — use case interaction, arg validation | mock `*Port` interfaces from `ports.go` (fake structs, no mockgen — matches existing `mocks_test.go` style) | 80%+ |
| Integration | `/mcp` happy path: `initialize` → `tools/list` (6 tools) → `tools/call` (`check_availability`) | `httptest.NewServer` + real `mcp.NewServer` + in-memory SQLite (`:memory:` via `modernc.org/sqlite`, WAL pragmas) | covered |
| Integration | auth wired: 401 → JSON-RPC `-32000`, 403 → `-32001` (REQ-AM-WIRED-004) | hit `/mcp` with/without `X-Caller-Id`, with client role calling staff-only tool | covered |
| E2E | mock LLM client (struct satisfying the SDK client surface) doing `initialize` → `tools/list` → `tools/call` against `httptest` server with real use cases + in-memory DB | `internal/mcp/e2e_test.go` | covered |
| Guard | `TestNoRepositoryImport` — grep `internal/mcp/*.go` for `internal/repository"` | compile-time-ish test | enforced |

All tests run with `-race` (AGENTS.md pre-flight). `go-sqlmock` (existing project pattern, `internal/repository/testutil_test.go`) is used for the booking-use-case interaction tests where in-memory SQLite is too heavy. In-memory SQLite is used for `/mcp` integration + e2e (real schema bootstrap via `db.NewDatabase` against a temp file, NOT `:memory:` — `modernc.org/sqlite` WAL needs a file path; use `t.TempDir()`).

## 9. PR strategy (2 chained PRs)

Per `work-unit-commits` + `chained-pr` skills, with **`feature-branch-chain`** (project default): each PR branches off the previous; only PR 2 targets `main`.

**Branches**: `feat/feat-mcp-transport-1` (off `main`) → PR 1 → `feat/feat-mcp-transport-2` (off `-1`) → PR 2 (base: `main`, but reviewable as the chain tip).

### PR 1 — Transport skeleton (~300–350 LOC)
**Behavior**: binary starts, validates loopback, binds `127.0.0.1:3000`, answers `initialize` + `tools/list` (0 tools), shuts down on SIGTERM. **No auth wired. No tools registered.**

| # | Commit (work unit) | Files | Verification |
|---|--------------------|-------|---------------|
| 1 | `feat(mcp): add loopback bind validator + unit tests` | `internal/mcp/loopback.go`, `loopback_test.go`, `doc.go` | `go test ./internal/mcp -run Loopback -race` green |
| 2 | `feat(mcp): add config loader + healthz + buildinfo` | `config.go`, `healthz.go`, `healthz_test.go`, `internal/buildinfo/buildinfo.go` | unit tests green |
| 3 | `feat(mcp): add streamable HTTP transport skeleton` | `server.go`, `transport.go`, `errors.go` (parse-error mapping only), `server_test.go` | `initialize`/`tools/list` against `httptest` green; 0 tools |
| 4 | `feat(mcp): add graceful shutdown + signal handling` | `shutdown.go`, `shutdown_test.go` | SIGTERM drains ≤10s test green |
| 5 | `feat(cmd): wire transport skeleton into main.go` | `cmd/mcp-server/main.go` (replace `_ =` block), `go.mod`/`go.sum` (SDK add) | `go build ./...` + `go test -race ./...` green; binary binds |

### PR 2 — Auth + 6 tools + e2e + doc fix (~270–370 LOC)
**Behavior**: `/mcp` carries a verified `Caller`; 6 tools dispatch to use cases; Spanish business errors surface; PRD "SSE" → "Streamable HTTP".

| # | Commit (work unit) | Files | Verification |
|---|--------------------|-------|---------------|
| 6 | `feat(mcp): wire auth middleware + JSON-RPC auth translator` | `auth_translator.go`, `auth_translator_test.go`, `main.go` (rbac + authMW) | 401/403 → JSON-RPC `-32000`/`-32001` test green (REQ-AM-WIRED-004) |
| 7 | `feat(usecase): add GetBusinessProfile use case` | `internal/application/usecase/get_business_profile.go`, `_test.go`, `main.go` | use case unit test green |
| 8 | `feat(mcp): register 6 booking/profile tools + error mapping` | `tools_booking.go`, `tools_profile.go`, `ports.go`, `errors.go` (full map), `_test.go` (×6 with mock ports) | `tools/list` returns 6; tools/call paths covered |
| 9 | `test(mcp): add /mcp integration + e2e mock-client` | `server_integration_test.go`, `e2e_test.go`, `no_repo_import_test.go` | integration + e2e + guard green |
| 10 | `docs(prd): SSE → Streamable HTTP (MCP 2025-11-25)` | `docs/PRD.md` (§3.1, §9.1, §1118, §1318), `docs/architecture/0007-server-config.md` | doc-only render check |

Risk if PR 2 > 400 LOC: split commit 9 (integration vs e2e) into its own chained PR (per `chained-pr` skill). Documented as Q-PR1.

## 10. Dependency audit

**Direct addition**: `github.com/modelcontextprotocol/go-sdk v1.2.0`.

**Transitive deps** (enumerated from the SDK's `go.mod`; final set confirmed at `go mod tidy` during apply — recorded in `tasks.md`):

| Module | License | Purpose |
|--------|--------|---------|
| `github.com/google/jsonschema` | Apache-2.0 | JSON-Schema inference from Go structs (tool input/output schemas) |
| `github.com/google/uuid` | Apache-2.0 | Session/request IDs (already indirect in go.mod) |
| (others pulled by `jsonschema`) | — | enumerated at apply time into `tasks.md` |

None add **runtime** dependencies beyond the Go binary itself — no cgo, no shared libraries, no separate binaries, no OS packages. The Go SDK is **compile-time only** like the existing `modernc.org/sqlite` (which is pure-Go, already accepted).

**ADR-0005 tension — explicit resolution**: ADR-0005 is scoped to *"scripts never install external **system tools** that the OS package manager manages"* (sqlite3 CLI, not Go modules). The project already depends on `modernc.org/sqlite` (a Go module) and `DATA-DOG/go-sqlmock` — the "no external deps" spirit is about **runtime/system tools**, not Go modules. The SDK is a compile-time Go module that compiles into the single static binary, identical in nature to `modernc.org/sqlite`. Therefore the addition is consistent with ADR-0005. **R5 mitigation**: this paragraph is the explicit reviewer-facing rationale; if the team disagrees, Plan B (hand-rolled JSON-RPC, zero new deps) is the fallback (~300 extra LOC).

## 11. Open implementation questions

- **Q-A1 (resolved)** — stateless transport vs 2025-11-25 protocol version: chose `Stateless:true` + advertise `2025-11-25` (the SDK max). Documented in §4. Satisfies REQ-MT-002 (GET→405) and REQ-MT-004 (version) simultaneously.
- **Q-A2** — JSON-RPC code for "tool not found": use SDK default `-32601` (Method not found). No custom code; matches SDK behaviour and REQ-MT-006 scenario verbatim.
- **Q-A3** — `/healthz` as separate handler vs SDK resource: separate `http.Handler` (decision §5). Simpler, no session semantics.
- **Q-A4** — Prometheus metrics endpoint: out of scope for this change (loopback-only, single client). Flag for Fase 3 with FTS5 search tools.
- **Q-O1** — does `internal/config/dotenv.go` ship with Fase 1, or does this change add it? Affects PR 1 commit sizing. Verify at apply.
- **Q-AM-1** — auth audit logger: dedicated `slog` sub-logger for journald filtering? Deferred; current "privileged access" line is sufficient.
- **Q-PR1** — if PR 2 > 400 LOC, split integration/e2e (commit 9) into a third chained PR.
- **Q-A5** — `0.0.0.0` explicit reject message: confirmed in §3 (ADR-0007 literal Spanish string).
- **Q-A6** — does the auth translator reuse the middleware's Spanish body or the spec's literal string? Use the spec's literal string for `-32000`/`-32001` (REQ-AM-WIRED-002/003 mandate them verbatim); the middleware already emits the same strings, so they coincide.

## 12. Risk mitigations (from proposal §6)

| # | Severity | Finding | Mitigation (concrete, this change) |
|---|----------|---------|-----|
| R1 | CRITICAL | SDK may not support 2026-07-28 (POST-only). | Target 2025-11-25 with `Stateless:true` (§4) — gives GET→405 now AND a no-code-change path to 2026-07-28. Plan B (hand-rolled) documented; gated at apply if `mcp.NewStreamableHTTPHandler` proves unworkable. |
| R2 | WARNING | PRD says "SSE" (deprecated). | In-scope doc fix in PR 2 commit 10 (§9). |
| R3 | RESOLVED | Hermes Streamable HTTP support. | No action — obs #664 verified. |
| R4 | SUGGESTION | POST-only vs POST+GET sessions. | `Stateless:true` already POST-only with GET→405 (§4). |
| R5 | WARNING | ADR-0005 "no external deps" tension. | §10 explicit trade-off: SDK is compile-time Go module, consistent with existing `modernc.org/sqlite`; ADR-0005 scoped to system runtime tools, not Go modules. Plan B fallback if contested. |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary introduced by this change. SDK HTTP serving is in-process; no `exec.Command`, no shell strings, no PR/VCS automation, no executable classification.

## Migration / Rollout

No data migration required. The `business_profile` lazy-init (`INSERT OR IGNORE`) already handles a fresh install. The new `get_business_profile` use case is pure read over an existing repo. Rollout is a binary replacement + service restart (Fase 5 installer is out of scope); no feature flag needed (Fase 2 DoD = serve the 6 tools on loopback).
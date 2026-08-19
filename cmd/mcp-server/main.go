// Package main implements the composition root for the MCP Appointments CRM server.
//
// The composition root is responsible for:
//   - Opening the SQLite database (WAL mode, busy_timeout=5000)
//   - Constructing all 9 repository implementations
//   - Sharing a single BookingValidator across Create and Reschedule use cases
//   - Wiring the 5 domain use cases with their correct dependency sets
//   - Serving the MCP streamable-HTTP endpoint (/mcp) and the health probe
//     (/healthz) on 127.0.0.1:3000, with a graceful 10s drain on SIGTERM/SIGINT
//   - Exiting 0 on a clean shutdown; 1 on any fatal startup or serve failure
//
// The transport skeleton (feat-mcp-transport PR 1) served the MCP endpoint
// with zero tools; PR 2 wires the authenticated transport (AuthMiddleware +
// JSON-RPC auth translator) and the tool registration that consumes the use
// cases below.
//
// This file is the first production caller of the 7-arg use case constructors
// introduced in feat-booking-validator-service (TASK-FU.3). No DI containers,
// reflection, or init() functions — all wiring is explicit in main().
//
// Design decisions documented per refactor-clean-architecture P4.1:
//
//	D1. DB path: ./data/appointments.db by default, override via MCP_DB_PATH env var
//	D2. Logger:   slog.Default() (writes to stderr); only NewAccountsRepo receives it
//	D3. Exit:     slog info + os.Exit(0) on clean shutdown; os.Exit(1) on DB
//	    failure, loopback violation, or listen/serve failure. A benign
//	    close-time DB error does NOT flip the exit code (systemd unit stays
//	    "successful" on a clean stop).
//	D4. bookingValidator interface: kept as narrow contract in
//	    internal/application/usecase/validator.go. The consumer-facing
//	    domain.BookingValidator interface is not declared because
//	    internal/domain/ has a zero-dependency rule (it cannot import
//	    internal/domain/entity/ — and ValidateBookingInput references entity
//	    types). Promotion to internal/domain/service/ is deferred until a
//	    third consumer appears (TASK-FU.3 resolution).
//
// D5. Transport: mcp.NewServer serves the streamable-HTTP handler behind
//
//	jsonParseGuard; AuthHandler adds AuthMiddleware + the JSON-RPC auth
//	translator (REQ-AM-WIRED-001/002). mcp.Run owns listen + serve +
//	graceful shutdown, so the composition root never touches the raw
//	listener.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/buildinfo"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
	"github.com/egkike/mcp-appointments-crm/internal/mcp"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

func main() {
	// os.Exit only here, where no defers are pending: run() owns the
	// database handle and always closes it before returning an error.
	if err := run(); err != nil {
		slog.Default().Error("mcp server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// D2: Logger from slog.Default() (writes to stderr).
	logger := slog.Default()

	// Resolve MCP server configuration: env vars > .env file > defaults.
	cfg, err := mcp.LoadConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	cfg.Version = buildinfo.Version
	cfg.Logger = logger

	// Fail fast on a non-loopback bind: the MCP endpoint must never be
	// reachable beyond this machine.
	if err := mcp.ValidateLoopback(cfg.Bind); err != nil {
		return fmt.Errorf("bind address is not loopback: %w", err)
	}

	// D1: Resolve database path — env var overrides default.
	dbPath := os.Getenv("MCP_DB_PATH")
	if dbPath == "" {
		dbPath = "./data/appointments.db"
	}

	// Open the SQLite database. NewDatabase creates the directory, verifies
	// pragmas, and runs initSchema — all idempotent.
	ctx := context.Background()
	database, err := db.NewDatabase(ctx, dbPath)
	if err != nil {
		// GGA W-3: the path stays out of the error string (security
		// checklist: no internal file paths in error messages) and is
		// logged as a structured field — operator-facing stderr/journal,
		// never sent to the MCP client.
		logger.Error("open database failed", "path", dbPath, "error", err)
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		// Intentional: log Close() error but don't change the exit code.
		// For a server-style binary, a clean shutdown that surfaces a
		// benign close-time error should NOT flip the systemd unit to
		// failed. Use os.Exit(1) only for fatal startup errors.
		if cerr := database.Close(); cerr != nil {
			logger.Error("failed to close database", "error", cerr)
		}
	}()

	// ── Construct all 9 repositories ──

	accountsRepo := repository.NewAccountsRepo(database.Conn, logger)
	bookingsRepo := repository.NewBookingsRepo(database.Conn)
	bizHoursExRepo := repository.NewBusinessHoursExceptionRepo(database.Conn)
	bizProfRepo := repository.NewBusinessProfileRepo(database.Conn)
	clientsRepo := repository.NewClientsRepo(database.Conn)
	pendingAlertsRepo := repository.NewPendingAlertsRepo(database.Conn)
	prosRepo := repository.NewProfessionalsRepo(database.Conn)
	schedulesRepo := repository.NewSchedulesRepo(database.Conn)
	servicesRepo := repository.NewServicesRepo(database.Conn)

	// TASK-FU.3: BookingValidator is stateless — construct once, share between
	// CreateBookingUseCase and RescheduleBookingUseCase. Both use cases accept
	// the narrow bookingValidator interface from validator.go; the concrete
	// *service.BookingValidator satisfies it structurally.
	bookingValidator := service.NewBookingValidator()

	// ── Wire the 5 domain use cases ──

	// 1-arg use cases: only Bookings repo needed.
	getBookingUC := usecase.NewGetBookingUseCase(bookingsRepo)
	cancelBookingUC := usecase.NewCancelBookingUseCase(bookingsRepo)

	// 7-arg use cases: bookings + 5 resolution repos + shared validator.
	// These are the first production callers of the expanded constructors
	// from feat-booking-validator-service.
	createBookingUC := usecase.NewCreateBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo,
		bizProfRepo, bizHoursExRepo, schedulesRepo,
		bookingValidator,
	)
	rescheduleBookingUC := usecase.NewRescheduleBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo,
		bizProfRepo, bizHoursExRepo, schedulesRepo,
		bookingValidator,
	)

	// CheckAvailability: different shape — takes an AvailabilityChecker
	// interface + a pre-assembled AvailabilityDeps struct.
	availabilityChecker := service.NewAvailabilityService()
	availabilityDeps := service.AvailabilityDeps{
		Services:                servicesRepo,
		Professionals:           prosRepo,
		BusinessProfile:         bizProfRepo,
		BusinessHoursExceptions: bizHoursExRepo,
		Schedules:               schedulesRepo,
		Bookings:                bookingsRepo,
	}
	checkAvailabilityUC := usecase.NewCheckAvailabilityUseCase(
		availabilityChecker, availabilityDeps,
	)

	// ── D3: Use cases are wired but not yet invoked ──
	//
	// Staged-PR bridge (GGA W-2): the repositories and use cases below are
	// constructed now so the composition root compiles against the real
	// dependency graph; the `_ =` block is removed atomically when the
	// tool-wiring commit of this PR registers the MCP tools that consume
	// them (ListClients, CreateAlert, etc.). check_availability is the one
	// use case already reachable at runtime: it feeds no tool yet, but the
	// authenticated /mcp endpoint below is live.
	_ = accountsRepo
	_ = clientsRepo
	_ = pendingAlertsRepo
	_ = getBookingUC
	_ = cancelBookingUC
	_ = createBookingUC
	_ = rescheduleBookingUC
	_ = checkAvailabilityUC

	// ── Auth: resolver + middleware + tool RBAC (design §3) ──
	//
	// Every /mcp request must carry X-Caller-Id; check_availability has no
	// RBAC entry (any authenticated caller — open set), the other five tools
	// restrict by role. RBAC keys on r.URL.Path, so the JSON-RPC auth
	// translator rewrites the path to the tool name for tools/call requests.
	resolver := auth.NewCallerResolver(database.Conn)
	rbac := auth.ToolRBAC{
		"create_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"cancel_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"reschedule_booking":   {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"get_booking":          {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"get_business_profile": {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
	}
	authMW := auth.NewAuthMiddleware(resolver, rbac, logger)

	// ── D5: Authenticated transport (tools registered by the tool-wiring
	// commit of this PR) ──

	mux := http.NewServeMux()
	mux.Handle("/healthz", mcp.Healthz(cfg.Version))
	mux.Handle("/mcp", mcp.NewServer(cfg).AuthHandler(authMW))

	httpSrv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Bind, cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout stays set (30s): the transport is JSON-only
		// (Stateless + JSONResponse, REQ-MT-002), so no long-lived SSE
		// stream can exist and the deadline is the fail-secure bound for a
		// stuck handler. mcp.Run's 10s drain owns shutdown (REQ-MT-010).
		// Revisit if SSE streaming is ever enabled (GGA W-2).
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("mcp server starting",
		"addr", httpSrv.Addr,
		"version", cfg.Version,
		"repos", 9,
		"usecases", 5,
		"booking_validator_shared", true,
	)

	result, err := mcp.Run(ctx, httpSrv, logger)
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	logger.Info("mcp server stopped cleanly",
		"drained", result.Drained,
		"force_closed", result.ForceClosed,
	)
	return nil
}

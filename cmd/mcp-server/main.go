// Package main implements the composition root for the MCP Appointments CRM server.
//
// The composition root is responsible for:
//   - Opening the SQLite database (WAL mode, busy_timeout=5000)
//   - Constructing all 9 repository implementations
//   - Sharing a single BookingValidator across Create and Reschedule use cases
//   - Wiring the 5 domain use cases with their correct dependency sets
//   - Serving the MCP streamable-HTTP endpoint (/mcp) and the health probe
//     (/healthz) on 127.0.0.1:3000, with a graceful 10s drain on SIGTERM/SIGINT
//   - Dispatching the CLI before serve mode: `admin tui` (identity & accounts,
//     ADR-0016) and the reserved `hermes chat` run as sub-commands; unknown
//     arguments fail fast with a semantic error instead of starting the server
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
//	D1. DB path: ~/.local/share/mcp-appointments-crm/reservas.db by default,
//	    resolved through os.UserHomeDir; override via MCP_DB_PATH env var.
//	    The default is absolute on purpose (ADR-0002: the data lives under the
//	    user's XDG data dir), so a manual `mcp-server admin tui` run without the
//	    env var cannot fork state into a CWD-relative file while the systemd
//	    unit (Environment=MCP_DB_PATH) points at the XDG layout.
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
//
// D6. CLI dispatch (admin-tui T1): main() resolves os.Args[1:] before run().
//
//	`--version` keeps its REQ-BVER-001 contract (handled first, untouched);
//	no argument means serve mode; `admin tui` and `hermes chat` are the only
//	recognized sub-commands (names frozen by ADR-0016 §5); anything else,
//	including incomplete sub-commands, is a domain.SemanticError that exits 1.
//	Sub-commands share loadConfig/openDatabase/newIdentityDeps with serve
//	mode so the two entry points cannot drift.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/buildinfo"
	"github.com/egkike/mcp-appointments-crm/internal/config"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
	"github.com/egkike/mcp-appointments-crm/internal/mcp"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

func main() {
	// REQ-BVER-001: --version prints buildinfo.Version and exits 0 before
	// any database or network setup.
	if wantsVersion(os.Args) {
		printVersion(os.Stdout)
		return
	}

	// #7 (hygiene-followups): one signal-bound context for the whole process.
	// SIGTERM/SIGINT cancel it and every command receives it, so the shared
	// SQLite open (openDatabase) is cancellation-aware and serve mode reuses the
	// same context instead of minting a private Background. The interactive
	// sub-commands use it ONLY for that shared open: their in-flight operator
	// writes must run to completion, so the admin flows keep their own
	// background context (see runAdminTUIProgram/runAdminTUIFlow). mcp.Run keeps
	// its own signal.Notify for the second-signal force-close; the two
	// registrations on the same signals coexist without clashing.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)

	// os.Exit only here, where no defers are pending: every runner owns its
	// database handle and always closes it before returning an error. stop() is
	// called explicitly (not deferred) so the signal handler is released on both
	// the clean and the fatal path without tripping exitAfterDefer.
	err := executeCLI(ctx, os.Args[1:], cliRunners{
		serve:      run,
		adminTUI:   runAdminTUI,
		hermesChat: runHermesChat,
	})
	stop()
	if err != nil {
		slog.Default().Error("mcp server failed", "error", err)
		os.Exit(1)
	}
}

// ── CLI dispatch (D6) ────────────────────────────────────────────────────
//
// The binary is invoked in three ways: bare (serve mode), `admin tui`
// (operator-facing identity/accounts TUI, ADR-0016) and the reserved
// `hermes chat`. Dispatch happens before any serve-mode wiring and never falls
// through from a sub-command into the server.

// commandKind identifies the top-level operation selected on the command line.
type commandKind int

const (
	// commandInvalid is the fail-secure zero value. Every rejected invocation
	// returns it alongside its error, so a parse bug can never default into
	// serve mode.
	commandInvalid commandKind = iota
	// commandServe is the bare invocation: the MCP streamable-HTTP server.
	commandServe
	// commandAdminTUI is `mcp-server admin tui` (ADR-0016 §5).
	commandAdminTUI
	// commandHermesChat is `mcp-server hermes chat` (reserved name, ADR-0016 §5).
	commandHermesChat
)

// usageHint is the single source of truth for the usage line. Every
// command-line error carries it (ADR-0016 §5). It stays on one line so it
// survives slog's structured output without escaped newlines.
const usageHint = "uso: mcp-server [--version] | mcp-server admin tui | mcp-server hermes chat"

// parseCommand maps the command-line arguments (os.Args[1:]) to the command
// the binary must run. `--version` never reaches this function: main()
// short-circuits it first (REQ-BVER-001, wantsVersion).
//
// Unknown, incomplete or extra arguments return a *domain.SemanticError
// instead of silently falling through to serve mode. Fail-fast on unknown
// arguments is required by ADR-0016 §5.
func parseCommand(args []string) (commandKind, error) {
	if len(args) == 0 {
		return commandServe, nil
	}

	switch args[0] {
	case "admin":
		return parseSubCommand(args, "admin", "tui", commandAdminTUI)
	case "hermes":
		return parseSubCommand(args, "hermes", "chat", commandHermesChat)
	default:
		return commandInvalid, invalidArgsError(fmt.Sprintf("argumento desconocido: %q", args[0]))
	}
}

// parseSubCommand validates the `<parent> <child>` shape shared by the two
// sub-command families and reports the semantic error the operator needs
// (missing child, unknown child, extra arguments).
func parseSubCommand(args []string, parent, child string, kind commandKind) (commandKind, error) {
	if len(args) == 1 {
		return commandInvalid, invalidArgsError(fmt.Sprintf(
			"sub-comando faltante para %q: el único sub-comando válido es %q", parent, child))
	}
	if args[1] != child {
		return commandInvalid, invalidArgsError(fmt.Sprintf(
			"sub-comando desconocido %q para %q: el único sub-comando válido es %q", args[1], parent, child))
	}
	if len(args) > 2 {
		return commandInvalid, invalidArgsError(fmt.Sprintf(
			"argumentos no esperados %q después de %q", args[2:], parent+" "+child))
	}
	return kind, nil
}

// invalidArgsError builds the semantic error for a command-line misuse. It
// always carries the usage hint.
func invalidArgsError(detail string) error {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: fmt.Sprintf("%s; %s", detail, usageHint),
	}
}

// commandRunner is the entry point of one top-level command. It receives the
// process signal context so the shared startup step (openDatabase) is
// cancellation-aware.
type commandRunner func(ctx context.Context) error

// cliRunners binds each command kind to its entry point. main() supplies the
// real runners; tests inject stubs so dispatch is asserted without a database
// or a live HTTP server.
type cliRunners struct {
	serve      commandRunner
	adminTUI   commandRunner
	hermesChat commandRunner
}

// executeCLI parses args and runs exactly one command. It is the single
// dispatch point of the binary: a sub-command can never fall through into
// serve mode, and an invalid invocation runs nothing at all.
func executeCLI(ctx context.Context, args []string, runners cliRunners) error {
	kind, err := parseCommand(args)
	if err != nil {
		return err
	}

	switch kind {
	case commandServe:
		return runners.serve(ctx)
	case commandAdminTUI:
		return runners.adminTUI(ctx)
	case commandHermesChat:
		return runners.hermesChat(ctx)
	default:
		// Defensive: parseCommand never yields commandInvalid with a nil
		// error, and this branch must not start the server if it ever did.
		return invalidArgsError(fmt.Sprintf("comando inválido: %d", kind))
	}
}

// wantsVersion reports whether the CLI was invoked with --version as the
// first positional argument. It is a pure helper so it can be unit-tested
// without touching os.Args or building a binary.
func wantsVersion(args []string) bool {
	return len(args) > 1 && args[1] == "--version"
}

// printVersion writes the current binary version to w.
func printVersion(w io.Writer) {
	_, _ = fmt.Fprintln(w, buildinfo.Version) //nolint:errcheck // best-effort write to stdout; caller exits immediately
}

func run(ctx context.Context) error {
	// D2: Logger from slog.Default() (writes to stderr).
	logger := slog.Default()

	// Resolve MCP server configuration: env vars > .env file > defaults.
	cfg, err := loadConfig(logger)
	if err != nil {
		return err
	}

	// Fail fast on a non-loopback bind: the MCP endpoint must never be
	// reachable beyond this machine.
	if err := mcp.ValidateLoopback(cfg.Bind); err != nil {
		return fmt.Errorf("bind address is not loopback: %w", err)
	}

	// D1: open the SQLite database (WAL, busy_timeout=5000) through the same
	// helper the sub-commands use (D6). ctx is the process signal context
	// created once in main(), so SIGTERM/SIGINT cancel the open like the rest of
	// serve mode instead of leaving it on a private Background.
	database, err := openDatabase(ctx, logger)
	if err != nil {
		return err
	}
	defer closeDatabase(database, logger)

	// Setup import (feat-setup-import): seed the DB from the wizard JSONs on
	// first boot. Runs in the single-threaded window before repo construction
	// and HTTP serving. No-op when the profile is already seeded; fatal when
	// the DB is fresh and the wizard output is missing or malformed.
	if err := config.SeedOnBoot(ctx, database.Conn, logger); err != nil {
		return fmt.Errorf("importar configuración inicial: %w", err)
	}

	// ── Construct repositories (only those the wired use cases consume) ──

	bookingsRepo := repository.NewBookingsRepo(database.Conn)
	bizHoursExRepo := repository.NewBusinessHoursExceptionRepo(database.Conn)
	bizProfRepo := repository.NewBusinessProfileRepo(database.Conn)
	prosRepo := repository.NewProfessionalsRepo(database.Conn)
	schedulesRepo := repository.NewSchedulesRepo(database.Conn)
	servicesRepo := repository.NewServicesRepo(database.Conn)

	// AccountsRepo + ClientsRepo share one construction site with the
	// `admin tui` sub-command (T1, ADR-0016 D3.5). Serve mode consumes only the
	// clients handle: account management deliberately stays outside the MCP
	// surface (ADR-0010), so the owner seed gateway is the TUI's job alone.
	// newIdentityDeps is the shared serve/admin-tui site: identity.professionals
	// duplicates prosRepo above and only the admin TUI consumes it, while
	// identity.accounts is routed into the "repos" inventory. The inventory
	// counts the serve-side handles, never this duplicate.
	identity := newIdentityDeps(database, logger)
	clientsRepo := identity.clients

	// TASK-FU.3: BookingValidator is stateless — construct once, share between
	// CreateBookingUseCase and RescheduleBookingUseCase. Both use cases accept
	// the narrow bookingValidator interface from validator.go; the concrete
	// *service.BookingValidator satisfies it structurally.
	bookingValidator := service.NewBookingValidator()

	// ── Wire the 5 domain use cases ──

	// Pending alerts adapter for booking lifecycle (create/cancel/reschedule).
	pendingAlertsRepo := repository.NewPendingAlertsRepo(database.Conn)

	// Startup telemetry source of truth: every repo handle serve mode wires
	// joins the name list inside wiredRepoInventory (9 handles, one per
	// repository implementation). A new handle that forgets the list makes the
	// log lie — keep it in sync, exactly like the MCP tool registry in
	// internal/mcp.
	repoCount := wiredRepoInventory()

	alertStore := usecase.NewEnsurePendingAlertsRepo(pendingAlertsRepo)

	// 1-arg use cases: only Bookings repo needed.
	getBookingUC := usecase.NewGetBookingUseCase(bookingsRepo)
	cancelBookingUC := usecase.NewCancelBookingUseCase(bookingsRepo, alertStore, logger)

	// 7-arg use cases: bookings + 5 resolution repos + shared validator + alert store.
	// These are the first production callers of the expanded constructors
	// from feat-booking-validator-service.
	createBookingUC := usecase.NewCreateBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo,
		bizProfRepo, bizHoursExRepo, schedulesRepo,
		clientsRepo, bookingValidator, alertStore, logger,
	)
	rescheduleBookingUC := usecase.NewRescheduleBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo,
		bizProfRepo, bizHoursExRepo, schedulesRepo,
		clientsRepo, bookingValidator, alertStore, logger,
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

	// 6th use case (Q3): get_business_profile wraps the singleton profile repo.
	getBusinessProfileUC := usecase.NewGetBusinessProfileUseCase(bizProfRepo)

	// PR 1 (Phase 1): caller-scoped FTS search use cases.
	searchClientsAdvancedUC := usecase.NewSearchClientsAdvancedUseCase(clientsRepo)
	searchServicesAdvancedUC := usecase.NewSearchServicesAdvancedUseCase(servicesRepo)

	// PR 2 (Phase 2): alert lifecycle use cases.
	getPendingAlertsUC := usecase.NewGetPendingAlertsUseCase(pendingAlertsRepo)
	markAlertAsSentUC := usecase.NewMarkAlertAsSentUseCase(pendingAlertsRepo)

	// PR 3 (Phase 3): loyalty report use case.
	getLoyaltyReportUC := usecase.NewGetLoyaltyReportUseCase(bookingsRepo)

	// Maintenance WRITE use cases (ADR-0015): the eight owner-only use cases
	// behind the Hermes operational maintenance tools. They share the repos and
	// the process logger (structured audit log per mutation lives in the use
	// case layer). The role gate is enforced twice: the owner-only ToolRBAC
	// entry below and auth.RequireRole(RoleOwner) inside every use case.
	updateBusinessProfileUC := usecase.NewUpdateBusinessProfileUseCase(bizProfRepo, logger)
	createServiceUC := usecase.NewCreateServiceUseCase(servicesRepo, logger)
	updateServiceUC := usecase.NewUpdateServiceUseCase(servicesRepo, logger)
	deleteServiceUC := usecase.NewDeleteServiceUseCase(servicesRepo, logger)
	createProfessionalUC := usecase.NewCreateProfessionalUseCase(prosRepo, logger)
	updateProfessionalUC := usecase.NewUpdateProfessionalUseCase(prosRepo, logger)
	upsertScheduleUC := usecase.NewUpsertScheduleUseCase(schedulesRepo, logger)
	deleteScheduleUC := usecase.NewDeleteScheduleUseCase(schedulesRepo, logger)

	// ── Auth: resolver + middleware + tool RBAC (design §3) ──
	//
	// Every /mcp request must carry X-Caller-Id. Three tools deliberately have
	// no entry here and enforce role downstream:
	//   - check_availability       → any authenticated caller
	//   - search_clients_advanced  → row scope by caller role (repository/clients.go)
	//   - search_services_advanced → auth.RequireRole(RoleOwner, RoleAdmin) in the use case
	// Every other tool is gated by the map below. RBAC keys on r.URL.Path, so
	// the JSON-RPC auth translator rewrites the path to the tool name for
	// tools/call requests. The eight maintenance tools are owner-only
	// (ADR-0015 Decision 2): the partial admin scope stays deferred, so no
	// admin role is granted here.
	resolver := auth.NewCallerResolver(database.Conn)
	rbac := auth.ToolRBAC{
		"create_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"cancel_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"reschedule_booking":   {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"get_booking":          {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"get_business_profile": {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"get_pending_alerts":   {auth.RoleOwner, auth.RoleAdmin},
		"mark_alert_as_sent":   {auth.RoleOwner, auth.RoleAdmin},
		"get_loyalty_report":   {auth.RoleOwner, auth.RoleAdmin},

		// Maintenance WRITE tools (ADR-0015): owner-only MVP.
		"update_business_profile": {auth.RoleOwner},
		"create_service":          {auth.RoleOwner},
		"update_service":          {auth.RoleOwner},
		"delete_service":          {auth.RoleOwner},
		"create_professional":     {auth.RoleOwner},
		"update_professional":     {auth.RoleOwner},
		"upsert_schedule":         {auth.RoleOwner},
		"delete_schedule":         {auth.RoleOwner},
	}
	authMW := auth.NewAuthMiddleware(resolver, rbac, logger)

	// ── D5: Authenticated transport (T-09: tools wired) ──
	//
	// The use cases back the MCP tools through the consumer ports
	// (internal/mcp/ports.go). A nil port would leave its tool unregistered;
	// the production composition injects all of them.
	srv := mcp.NewServer(mcp.Config{
		Version:                cfg.Version,
		Logger:                 logger,
		CheckAvailability:      checkAvailabilityUC,
		CreateBooking:          createBookingUC,
		GetBooking:             getBookingUC,
		CancelBooking:          cancelBookingUC,
		RescheduleBooking:      rescheduleBookingUC,
		GetBusinessProfile:     getBusinessProfileUC,
		SearchClientsAdvanced:  searchClientsAdvancedUC,
		SearchServicesAdvanced: searchServicesAdvancedUC,
		GetPendingAlerts:       getPendingAlertsUC,
		MarkAlertAsSent:        markAlertAsSentUC,
		GetLoyaltyReport:       getLoyaltyReportUC,

		UpdateBusinessProfile: updateBusinessProfileUC,
		CreateService:         createServiceUC,
		UpdateService:         updateServiceUC,
		DeleteService:         deleteServiceUC,
		CreateProfessional:    createProfessionalUC,
		UpdateProfessional:    updateProfessionalUC,
		UpsertSchedule:        upsertScheduleUC,
		DeleteSchedule:        deleteScheduleUC,
	})

	mux := http.NewServeMux()
	mux.Handle("/healthz", mcp.Healthz(cfg.Version))
	mux.Handle("/mcp", srv.AuthHandler(authMW))

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
		"repos", repoCount,
		// "usecases" reads the MCP tool registry: each tool is backed by
		// exactly one non-nil Config port, so the registered-tool count is
		// the wired use-case count 1:1 (ADR-0015/transport design). The key
		// stays "usecases" because docs/ops grep it.
		"usecases", srv.ToolCount(),
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

// wiredRepoInventory reports how many repository handles serve mode wires, one
// per repository implementation, and is the single source of truth for the
// startup telemetry "repos" count.
//
// The count is derived from the name list below with len, so the list — not a
// hard-coded literal — is what the operator reads, audits and updates. Adding a
// repository handle to the wiring in run() MUST also join this list, exactly
// like the MCP tool registry in internal/mcp
// (TestServerToolCountTracksRegisteredTools).
//
// This is an auditable maintenance contract, not a compiler-enforced one: the
// previous typed-parameter shape claimed a compile-time guarantee it did not
// provide, since the pointer parameters were never read and a new handle added
// only to the wiring still compiled. TestWiredRepoInventoryContract pins both
// the count and the documented handle set so drift is caught at test time.
func wiredRepoInventory() int {
	return len([]string{
		"bookings", "business_hours_exceptions", "business_profile",
		"professionals", "schedules", "services", "pending_alerts",
		"clients", "accounts",
	})
}

// runHermesChat is the entry point of `mcp-server hermes chat`, a name reserved
// by ADR-0016 §5 for the local Hermes chat (ADR-0012).
//
// T1 wires the sub-command only: it validates the shared startup dependencies
// exactly like serve mode does and then fails fast, so the reserved name never
// silently degrades into the MCP server.
func runHermesChat(ctx context.Context) error {
	deps, err := openCommandDependencies(ctx)
	if err != nil {
		return err
	}
	defer deps.close()

	return &domain.SemanticError{
		Code:    domain.ErrCodeInternal,
		Message: "el chat de Hermes todavía no está implementado (nombre reservado por ADR-0016 §5)",
	}
}

// ── Shared startup helpers (D6) ──────────────────────────────────────────
//
// Serve mode and the sub-commands must fail for the same reasons, so the
// startup sequence is factored into the helpers below instead of being
// duplicated per entry point.

// loadConfig resolves the runtime configuration (env vars > .env file >
// defaults) and attaches the process logger and build version.
func loadConfig(logger *slog.Logger) (mcp.Config, error) {
	cfg, err := mcp.LoadConfig()
	if err != nil {
		return mcp.Config{}, fmt.Errorf("load configuration: %w", err)
	}
	cfg.Version = buildinfo.Version
	cfg.Logger = logger
	return cfg, nil
}

// commandDependencies bundles the startup dependencies every top-level command
// shares: the resolved configuration, the process logger and the open SQLite
// handle. The identity repositories are built from it by newIdentityDeps.
type commandDependencies struct {
	config   mcp.Config
	logger   *slog.Logger
	database *db.DB
}

// openCommandDependencies validates and opens the config and database
// dependencies shared by the sub-commands, in the same order serve mode uses
// for those two steps (loopback validation stays serve-only: the TUI does not
// depend on the HTTP transport, ADR-0016 Decision 3.5). The caller MUST call
// close when done.
func openCommandDependencies(ctx context.Context) (*commandDependencies, error) {
	logger := slog.Default()

	cfg, err := loadConfig(logger)
	if err != nil {
		return nil, err
	}

	database, err := openDatabase(ctx, logger)
	if err != nil {
		return nil, err
	}

	return &commandDependencies{config: cfg, logger: logger, database: database}, nil
}

// close releases the database handle held by the command.
func (d *commandDependencies) close() {
	closeDatabase(d.database, d.logger)
}

// resolveDBPath resolves the SQLite file the process must open. MCP_DB_PATH is
// an explicit override and wins whenever it is non-empty (D1). Without it, the
// path is the XDG data layout under the user home
// (~/.local/share/mcp-appointments-crm/reservas.db), which is the same file the
// systemd/launchd unit points at through Environment=MCP_DB_PATH. The absolute
// default is what removes the CWD-relative fork: a manual run and the service
// share one database instead of silently diverging.
//
// XDG_DATA_HOME is deliberately not honored: the service units pin MCP_DB_PATH
// to this home layout, so an XDG-redirected manual run would open a different
// SQLite file than the service (split-brain bookings). Operators who need a
// custom location set MCP_DB_PATH.
//
// A home directory that cannot be resolved is a hard error, never a fallback:
// guessing a path here is exactly the split-brain this function exists to
// prevent.
func resolveDBPath() (string, error) {
	if dbPath := os.Getenv("MCP_DB_PATH"); dbPath != "" {
		return dbPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// The home directory stays out of the returned error (security
		// checklist: no internal file paths in client-facing errors).
		// os.UserHomeDir reports only the missing variable, and the caller
		// logs the cause as a structured field for the operator.
		return "", fmt.Errorf("resolving default DB path: %w", err)
	}
	return filepath.Join(home, ".local", "share", "mcp-appointments-crm", "reservas.db"), nil
}

// openDatabase resolves the SQLite path (D1: the MCP_DB_PATH env var overrides
// the XDG default resolved by resolveDBPath) and opens the database.
// NewDatabase creates the directory, verifies pragmas, and runs initSchema — all
// idempotent.
func openDatabase(ctx context.Context, logger *slog.Logger) (*db.DB, error) {
	dbPath, err := resolveDBPath()
	if err != nil {
		logger.Error("resolve default database path failed", "error", err)
		return nil, fmt.Errorf("open database: %w", err)
	}

	database, err := db.NewDatabase(ctx, dbPath)
	if err != nil {
		// GGA W-3: the path stays out of the error string (security
		// checklist: no internal file paths in error messages) and is
		// logged as a structured field — operator-facing stderr/journal,
		// never sent to the MCP client.
		logger.Error("open database failed", "path", dbPath, "error", err)
		return nil, fmt.Errorf("open database: %w", err)
	}
	return database, nil
}

// closeDatabase closes the SQLite handle. Intentional: a close-time error is
// logged but never returned. For a server-style binary, a clean shutdown that
// surfaces a benign close-time error should NOT flip the systemd unit to
// failed; os.Exit(1) is reserved for fatal startup errors (D3).
func closeDatabase(database *db.DB, logger *slog.Logger) {
	if cerr := database.Close(); cerr != nil {
		logger.Error("failed to close database", "error", cerr)
	}
}

// identityDeps bundles the account-facing repositories. They are constructed at
// this single site so serve mode and the `admin tui` sub-command cannot drift
// in construction style (T1, ADR-0016 D3.5). The professionals handle backs the
// Add Staff picker (T3).
type identityDeps struct {
	accounts      *repository.AccountsRepo
	clients       *repository.ClientsRepo
	professionals *repository.ProfessionalsRepo
}

// newIdentityDeps constructs the accounts, clients and professionals
// repositories from an already-open database. It does not open connections or
// run migrations.
func newIdentityDeps(database *db.DB, logger *slog.Logger) identityDeps {
	return identityDeps{
		accounts:      repository.NewAccountsRepo(database.Conn, logger),
		clients:       repository.NewClientsRepo(database.Conn),
		professionals: repository.NewProfessionalsRepo(database.Conn),
	}
}

// Package main implements the composition root for the MCP Appointments CRM server.
//
// The composition root is responsible for:
//   - Opening the SQLite database (WAL mode, busy_timeout=5000)
//   - Constructing all 9 repository implementations
//   - Sharing a single BookingValidator across Create and Reschedule use cases
//   - Wiring the 5 domain use cases with their correct dependency sets
//   - Exiting 0 on success (the SSE server and MCP transport are added by
//     feat-mcp-transport, a separate SDD change)
//
// This file is the first production caller of the 7-arg use case constructors
// introduced in feat-booking-validator-service (TASK-FU.3). No DI containers,
// reflection, or init() functions — all wiring is explicit in main().
//
// Design decisions documented per refactor-clean-architecture P4.1:
//
//	D1. DB path: ./data/appointments.db by default, override via MCP_DB_PATH env var
//	D2. Logger:   slog.Default() (writes to stderr); only NewAccountsRepo receives it
//	D3. Exit:     slog info + os.Exit(0) on success; os.Exit(1) on DB failure
//	D4. bookingValidator interface: kept as narrow contract in
//	    internal/application/usecase/validator.go. The consumer-facing
//	    domain.BookingValidator interface is not declared because
//	    internal/domain/ has a zero-dependency rule (it cannot import
//	    internal/domain/entity/ — and ValidateBookingInput references entity
//	    types). Promotion to internal/domain/service/ is deferred until a
//	    third consumer appears (TASK-FU.3 resolution).
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

func main() {
	// D1: Resolve database path — env var overrides default.
	dbPath := os.Getenv("MCP_DB_PATH")
	if dbPath == "" {
		dbPath = "./data/appointments.db"
	}

	// D2: Logger from slog.Default() (writes to stderr).
	logger := slog.Default()

	// Open the SQLite database. NewDatabase creates the directory, verifies
	// pragmas, and runs initSchema — all idempotent.
	ctx := context.Background()
	database, err := db.NewDatabase(ctx, dbPath)
	if err != nil {
		logger.Error("failed to open database", "error", err, "path", dbPath)
		os.Exit(1)
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

	// ── D3: Composition successfully wired ──
	//
	// The variables accountsRepo, clientsRepo, and pendingAlertsRepo are
	// constructed but not yet wired to any use case — they will be consumed
	// by future use cases (ListClients, CreateAlert, etc.) in the
	// feat-mcp-transport SDD.
	//
	// All use case variables are also unused at this stage: this composition
	// root only verifies the wiring compiles and exits. Actual invocation
	// comes when the MCP transport layer (feat-mcp-transport) connects SSE
	// endpoints to these use cases.
	_ = accountsRepo
	_ = clientsRepo
	_ = pendingAlertsRepo
	_ = getBookingUC
	_ = cancelBookingUC
	_ = createBookingUC
	_ = rescheduleBookingUC
	_ = checkAvailabilityUC

	logger.Info("composition root wired successfully",
		"repos", 9,
		"usecases", 5,
		"booking_validator_shared", true,
	)
}

package mcp

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/egkike/mcp-appointments-crm/internal/buildinfo"
	"github.com/egkike/mcp-appointments-crm/internal/config"
)

// Default listener values for the MCP server (REQ-MT-013 / ADR-0007).
const (
	defaultBind = "127.0.0.1"
	defaultPort = "3000"
)

// Config holds the MCP server configuration. Bind and Port are resolved by
// LoadConfig (env > .env > default); Version and Logger are injected by the
// composition root. The port fields carry the application use cases that back
// the MCP tools (T-09, ADR-0015): a nil port keeps the skeleton behavior (the
// corresponding tool is not registered), which keeps transport-level tests
// green. internal/mcp only consumes the ports through the interfaces declared
// in ports.go; the composition root injects concrete *usecase values.
type Config struct {
	Bind    string
	Port    string
	Version string
	Logger  *slog.Logger

	CheckAvailability      CheckAvailabilityPort
	CreateBooking          CreateBookingPort
	GetBooking             GetBookingPort
	CancelBooking          CancelBookingPort
	RescheduleBooking      RescheduleBookingPort
	GetBusinessProfile     BusinessProfilePort
	SearchClientsAdvanced  SearchClientsAdvancedPort
	SearchServicesAdvanced SearchServicesAdvancedPort

	// SearchProfessionalsAdvanced is the readable-by-any-authenticated-caller
	// professionals catalog (feat-owner-chat S7a). Like the two search ports
	// above it has no ToolRBAC entry; unlike SearchServicesAdvanced the use case
	// does NOT narrow the role, because the booking flow is the client's.
	SearchProfessionalsAdvanced SearchProfessionalsAdvancedPort

	// GetProfessionalSchedules is the owner-only agenda read (feat-owner-chat
	// S7b). Unlike the port above it has a ToolRBAC entry in the composition
	// root and the use case re-asserts auth.RequireRole(RoleOwner): the
	// double enforcement closes the delete_schedule read-gap without widening
	// the wire for clients (design §8.3 D5).
	GetProfessionalSchedules GetProfessionalSchedulesPort

	GetPendingAlerts GetPendingAlertsPort
	MarkAlertAsSent  MarkAlertAsSentPort
	GetLoyaltyReport GetLoyaltyReportPort

	// RegisterClient is the anonymous self-registration port (feat-whatsapp-bot
	// D2). It is the only port reached without a resolved caller: its tool has no
	// ToolRBAC entry and the use case takes no caller, so the phone travels as an
	// explicit argument sourced from the X-Caller-Id header.
	RegisterClient RegisterClientPort

	// Maintenance WRITE ports (ADR-0015): owner-only in the composition root.
	UpdateBusinessProfile UpdateBusinessProfilePort
	CreateService         CreateServicePort
	UpdateService         UpdateServicePort
	DeleteService         DeleteServicePort
	CreateProfessional    CreateProfessionalPort
	UpdateProfessional    UpdateProfessionalPort
	UpsertSchedule        UpsertSchedulePort
	DeleteSchedule        DeleteSchedulePort

	// Business-hours exception ports (feat-hours-exception-tool): owner-only
	// like the maintenance writes above, and sharing the same
	// BusinessHoursExceptionRepo the availability read path already consumes.
	CreateHoursException CreateHoursExceptionPort
	DeleteHoursException DeleteHoursExceptionPort
	ListHoursExceptions  ListHoursExceptionsPort
}

// LoadConfig resolves MCP_BIND and MCP_PORT with ADR-0007 precedence:
// environment variables > ~/.config/mcp-appointments-crm/.env > defaults
// 127.0.0.1:3000. Version falls back to buildinfo.Version ("dev" when unset).
//
// It returns an error when the .env tier is configured but unreadable (fail
// fast instead of silently binding defaults); a missing .env file is not an
// error — the tier is optional.
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// No home directory means the .env tier is unavailable; the tier is
		// optional by design (ADR-0007 §D5), so resolve from environment
		// variables and defaults only. Do NOT build a relative path here:
		// filepath.Join("", ".config", ...) yields ./.config/... and could load
		// an unintended file from the process working directory.
		return loadConfigFrom("")
	}
	return loadConfigFrom(filepath.Join(home, ".config", "mcp-appointments-crm", ".env"))
}

// loadConfigFrom resolves the configuration using envPath as the .env file.
// It is split from LoadConfig so tests can point at a temporary file. An empty
// envPath disables the .env tier (the no-home case): never read a relative path.
func loadConfigFrom(envPath string) (Config, error) {
	vars := make(map[string]string)
	if envPath != "" {
		loaded, err := config.LoadDotEnv(envPath)
		if err != nil {
			return Config{}, fmt.Errorf("load dotenv: %w", err)
		}
		vars = loaded
	}
	return Config{
		Bind:    firstNonEmpty(os.Getenv("MCP_BIND"), vars["MCP_BIND"], defaultBind),
		Port:    firstNonEmpty(os.Getenv("MCP_PORT"), vars["MCP_PORT"], defaultPort),
		Version: buildinfo.Version,
	}, nil
}

// firstNonEmpty returns the first non-empty value, or "" when all are empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

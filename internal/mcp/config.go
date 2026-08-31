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
// composition root. The six port fields carry the application use cases that
// back the MCP tools (T-09): a nil port keeps the skeleton behavior (the
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
	GetPendingAlerts       GetPendingAlertsPort
	MarkAlertAsSent        MarkAlertAsSentPort
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
	if err != nil {
		// No home directory means no .env tier is available; the tier is
		// optional by design (ADR-0007 §D5), so fall through to defaults.
		home = ""
	}
	envPath := filepath.Join(home, ".config", "mcp-appointments-crm", ".env")
	return loadConfigFrom(envPath)
}

// loadConfigFrom resolves the configuration using envPath as the .env file.
// It is split from LoadConfig so tests can point at a temporary file.
func loadConfigFrom(envPath string) (Config, error) {
	vars, err := config.LoadDotEnv(envPath)
	if err != nil {
		return Config{}, fmt.Errorf("load dotenv: %w", err)
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

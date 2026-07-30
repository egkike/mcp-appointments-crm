package service

import (
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// ParseBusinessTimezone loads an IANA timezone location by name.
// Returns an error wrapping domain.ErrInvalidInput if the name is empty or invalid.
func ParseBusinessTimezone(s string) (*time.Location, error) {
	if s == "" {
		return nil, fmt.Errorf("la zona horaria no puede estar vacía: %w", domain.ErrInvalidInput)
	}
	loc, err := time.LoadLocation(s)
	if err != nil {
		return nil, fmt.Errorf("la zona horaria %q no es válida: %w", s, domain.ErrInvalidInput)
	}
	return loc, nil
}

// ParseStartDatetime parses an RFC3339 datetime string in the given timezone.
// Uses time.RFC3339Nano which accepts 0-9 fractional-second digits in a single
// layout, covering both bare RFC3339 ("2026-01-04T10:00:00Z") and the
// millisecond form ("2026-01-04T10:00:00.000Z") that MCP clients commonly send.
// Returns an error wrapping domain.ErrInvalidInput if the format is invalid.
func ParseStartDatetime(input string, loc *time.Location) (time.Time, error) {
	dt, err := time.ParseInLocation(time.RFC3339Nano, input, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("el formato de fecha/hora %q no es válido (se espera RFC3339): %w", input, domain.ErrInvalidInput)
	}
	return dt, nil
}

// hhmmToMinutes converts a "HH:MM" string to total minutes since midnight.
// Returns an error wrapping domain.ErrInvalidInput if the format is invalid.
func hhmmToMinutes(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("formato HH:MM inválido %q: %w", s, domain.ErrInvalidInput)
	}
	return t.Hour()*60 + t.Minute(), nil
}

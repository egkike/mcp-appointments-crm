package repository

import (
	"fmt"
	"time"
)

// ParseBusinessTimezone loads an IANA timezone location by name.
// Returns an error if the timezone name is invalid or empty.
// Does NOT silently default to UTC — callers must handle the error.
func ParseBusinessTimezone(s string) (*time.Location, error) {
	if s == "" {
		return nil, fmt.Errorf("la zona horaria no puede estar vacía: %w", ErrInvalidInput)
	}
	loc, err := time.LoadLocation(s)
	if err != nil {
		return nil, fmt.Errorf("la zona horaria %q no es válida: %w", s, ErrInvalidInput)
	}
	return loc, nil
}

// ParseStartDatetime parses an RFC3339 datetime string in the given timezone location.
// Uses time.ParseInLocation to respect the provided timezone context.
// Also accepts millisecond-precision format ("2006-01-02T15:04:05.000Z07:00").
// Returns an error if the input format is invalid.
func ParseStartDatetime(input string, loc *time.Location) (time.Time, error) {
	dt, err := time.ParseInLocation(time.RFC3339, input, loc)
	if err != nil {
		dt, err = time.ParseInLocation("2006-01-02T15:04:05.000Z07:00", input, loc)
		if err != nil {
			return time.Time{}, fmt.Errorf("el formato de fecha/hora %q no es válido (se espera RFC3339): %w", input, ErrInvalidInput)
		}
	}
	return dt, nil
}

// storageTimeLayout is the canonical storage format for UTC timestamps with
// millisecond precision. The SQL strftime format ('%Y-%m-%dT%H:%M:%fZ') used
// in bookings.go and professionals.go MUST match this layout.
const storageTimeLayout = "2006-01-02T15:04:05.000Z"

// FormatStorage formats a time.Time as an ISO 8601 UTC string with millisecond precision.
// The output format is "2006-01-02T15:04:05.000Z" (e.g., "2026-07-13T13:30:00.000Z").
// The input is converted to UTC before formatting.
func FormatStorage(t time.Time) string {
	return t.UTC().Format(storageTimeLayout)
}

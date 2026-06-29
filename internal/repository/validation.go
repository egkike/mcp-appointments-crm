package repository

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Shared validation regexes used across multiple repository files.
// Consolidated here to avoid duplication and ensure consistency.
var (
	// datePattern matches YYYY-MM-DD strictly (no time component).
	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	// timeHHMMRe matches HH:MM in strict 24-hour format (HH: 00-23, MM: 00-59).
	timeHHMMRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

	// ftsQueryRe matches characters NOT allowed in FTS5 queries.
	// Allows Unicode letters (\p{L}), Unicode digits (\p{N}), whitespace, and
	// hyphens. This ensures Spanish accented terms (e.g. "alergía", "María")
	// pass validation while FTS5 operator characters (*, +, -, NOT, OR, AND)
	// are rejected.
	ftsQueryRe = regexp.MustCompile(`[^\p{L}\p{N}\s\-]`)
)

// validateExceptionDate checks that date is a valid YYYY-MM-DD string
// representing a real calendar date (rejects "2026-02-30", "2026-13-45", etc.).
// Returns ErrInvalidInput wrapping error if malformed.
func validateExceptionDate(date string) error {
	if !datePattern.MatchString(date) {
		return fmt.Errorf("la fecha debe tener formato YYYY-MM-DD, se recibió: %q: %w",
			date, ErrInvalidInput)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("la fecha %q no es una fecha válida: %w",
			date, ErrInvalidInput)
	}
	return nil
}

// validateFTSQuery checks that a full-text search query is non-empty and
// does not contain FTS5 operator characters.
// Returns ErrInvalidInput wrapping error if invalid.
func validateFTSQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("consulta vacía: %w", ErrInvalidInput)
	}
	if ftsQueryRe.MatchString(query) {
		return fmt.Errorf("la consulta contiene caracteres no permitidos: %w", ErrInvalidInput)
	}
	return nil
}

// validateBusinessHoursJSON checks that s is a valid JSON object (not null,
// array, or primitive). Empty string is allowed (optional field).
func validateBusinessHoursJSON(s string) error {
	if s == "" {
		return nil
	}
	if !json.Valid([]byte(s)) {
		return fmt.Errorf("el campo business_hours debe ser JSON válido: %w", ErrInvalidInput)
	}
	// Verify it's an object, not an array or primitive.
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("el campo business_hours debe ser un objeto JSON: %w", ErrInvalidInput)
	}
	return nil
}

// validateTimezone checks that tz is a valid IANA timezone name.
// Empty string is allowed (optional field, defaults to UTC at DB level).
func validateTimezone(tz string) error {
	if tz == "" {
		return nil
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("la zona horaria %q no es válida: %w", tz, ErrInvalidInput)
	}
	return nil
}

// validateAcceptedPaymentMethodsJSON checks that s is a valid JSON array of
// non-empty strings. Rejects JSON "null", primitives, and objects.
func validateAcceptedPaymentMethodsJSON(s string) error {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return fmt.Errorf("los métodos de pago deben ser un array JSON válido: %w", ErrInvalidInput)
	}
	var methods []string
	if err := json.Unmarshal([]byte(s), &methods); err != nil {
		return fmt.Errorf("los métodos de pago deben ser un array JSON válido: %w", ErrInvalidInput)
	}
	for i, m := range methods {
		if m == "" {
			return fmt.Errorf("el método de pago en la posición %d está vacío: %w", i, ErrInvalidInput)
		}
	}
	return nil
}

package repository

// These validation regexes/helpers live in the repository layer (not in
// internal/validation/) because they are only used to validate data
// arriving from the SQLite layer (date strings, FTS5 query syntax).
// Domain and application layers do their own validation at the
// entity/use-case boundary. If cross-layer validation is ever needed,
// this can be promoted to internal/validation/ — for now, single-call-site
// helpers belong with their callers.

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
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

	// ftsOperatorRe matches whole-word FTS5 operators that would alter
	// query semantics (AND, OR, NOT) or a +/- prefix after whitespace or
	// another operator. Inline hyphens inside a single term remain allowed.
	ftsOperatorRe = regexp.MustCompile(`(?i)(^|\s)(NOT|AND|OR)(\s|$)|[\s\-+]\s*[-+]`)
)

// validateExceptionDate checks that date is a valid YYYY-MM-DD string
// representing a real calendar date (rejects "2026-02-30", "2026-13-45", etc.).
// Returns domain.ErrInvalidInput wrapping error if malformed.
func validateExceptionDate(date string) error {
	if !datePattern.MatchString(date) {
		return fmt.Errorf("la fecha debe tener formato YYYY-MM-DD, se recibió: %q: %w",
			date, domain.ErrInvalidInput)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("la fecha %q no es una fecha válida: %w",
			date, domain.ErrInvalidInput)
	}
	return nil
}

// validateFTSQuery checks that a full-text search query is non-empty and
// does not contain FTS5 operator characters or whole-word operators that
// would alter the query semantics. Queries are bound with ? placeholders,
// so this validation is purely about preventing unintended FTS5 syntax.
// Returns domain.ErrInvalidInput wrapping error if invalid.
func validateFTSQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("consulta vacía: %w", domain.ErrInvalidInput)
	}
	if ftsQueryRe.MatchString(query) {
		return fmt.Errorf("la consulta contiene caracteres no permitidos: %w", domain.ErrInvalidInput)
	}
	if isFTSOperatorQuery(query) {
		return fmt.Errorf("la consulta contiene operadores FTS5 no permitidos: %w", domain.ErrInvalidInput)
	}
	return nil
}

// isFTSOperatorQuery rejects whole-word FTS5 operators and leading +/-.
// Inline hyphens (e.g. "geo-local") are still allowed because the tokenizer
// keeps the term as one token; leading/trailing or repeated operators are not.
func isFTSOperatorQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if len(trimmed) > 0 {
		first := trimmed[0]
		if first == '+' || first == '-' {
			return true
		}
	}
	if ftsOperatorRe.MatchString(query) {
		return true
	}
	return false
}

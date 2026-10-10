package repository

// These validation regexes/helpers live in the repository layer (not in
// internal/validation/) because the repository adapters are where untrusted
// upstream LLM input first reaches SQLite: FTS5 search queries (MATCH clauses)
// and exception date strings. They reject malformed or unsafe input before it
// is bound to a prepared statement. Domain and application layers still enforce
// entity/use-case invariants at their own boundary. Keeping these helpers here
// avoids the repository adapter importing internal/validation/ (data-access
// spec), and transport must not import this package (REQ-MT-012), so the small
// overlap of rules is intentional.

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// Shared validation regexes used across multiple repository files.
// Consolidated here to avoid duplication and ensure consistency.
var (
	// datePattern matches YYYY-MM-DD strictly (no time component).
	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	// timeHHMMRe matches HH:MM in strict 24-hour format (HH: 00-23, MM: 00-59).
	timeHHMMRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

	// ftsQueryRe matches characters NOT allowed in FTS5 queries. It allows
	// Unicode letters (\p{L}), Unicode digits (\p{N}), whitespace, and hyphens,
	// so accented Spanish terms (e.g. "alergía", "María") pass. It rejects the
	// remaining FTS5 operator characters (* and +) outright; hyphens are allowed
	// here and then narrowed to term-interior positions by ftsTermBoundaryRe.
	ftsQueryRe = regexp.MustCompile(`[^\p{L}\p{N}\s\-]`)

	// ftsTermBoundaryRe matches hyphens at a term boundary: at the start of the
	// string or a token, before whitespace, or at the end of the string. Native
	// FTS5 treats these as syntax ("geo-" → syntax error, "a- b" → no such
	// column, "-geo" → column syntax), so they are rejected together with the
	// disallowed-character set while term-interior hyphens ("geo-local") remain
	// valid.
	ftsTermBoundaryRe = regexp.MustCompile(`(^|\s)-|-\s|-$`)

	// ftsOperatorRe matches whole-word FTS5 operators that would alter query
	// semantics (AND, OR, NOT) or a repeated/paired hyphen operator after
	// whitespace or another hyphen. Inline hyphens inside a single term remain
	// allowed. A literal '+' never reaches this regex because ftsQueryRe rejects
	// it first, so it carries no '+' branch.
	ftsOperatorRe = regexp.MustCompile(`(?i)(^|\s)(NOT|AND|OR)(\s|$)|[\s\-]\s*-`)
)

// validateExceptionDate checks that date is a valid YYYY-MM-DD string
// representing a real calendar date (rejects "2026-02-30", "2026-13-45", etc.).
// Each failure returns *domain.SemanticError{Code: ErrCodeInvalidInput} with a
// clean Spanish message; the Cause keeps domain.ErrInvalidInput reachable
// through errors.Is, and internal/mcp maps the semantic error to JSON-RPC
// -32002. The raw date is echoed only into Cause, never into the LLM-facing
// Message.
func validateExceptionDate(date string) error {
	if !datePattern.MatchString(date) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la fecha debe tener formato YYYY-MM-DD.",
			Cause:   fmt.Errorf("la fecha debe tener formato YYYY-MM-DD, se recibió: %q: %w", date, domain.ErrInvalidInput),
		}
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la fecha no es una fecha válida (formato YYYY-MM-DD).",
			Cause:   fmt.Errorf("la fecha %q no es una fecha válida (formato YYYY-MM-DD): %w", date, domain.ErrInvalidInput),
		}
	}
	return nil
}

// MaxFTSQueryLen bounds the length of an FTS5 query string. This limits
// the memory and CPU surface of MATCH clauses against untrusted upstream
// input without affecting legitimate searches.
const MaxFTSQueryLen = 200

// validateFTSQuery checks that a full-text search query is non-empty, does not
// exceed MaxFTSQueryLen, does not contain FTS5 operator characters or
// term-boundary hyphens, and does not contain whole-word operators that would
// alter the query semantics. Queries are bound with ? placeholders, so this
// validation is purely about preventing unintended FTS5 syntax.
//
// Each failure returns *domain.SemanticError{Code: ErrCodeInvalidInput} with a
// clean user-facing Spanish message (F2); the Cause keeps
// domain.ErrInvalidInput reachable through errors.Is/errors.As, and the
// semantic error itself is what internal/mcp maps to JSON-RPC -32002 instead of
// the generic -32603.
func validateFTSQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la consulta de búsqueda está vacía.",
			Cause:   fmt.Errorf("consulta vacía: %w", domain.ErrInvalidInput),
		}
	}
	if utf8.RuneCountInString(query) > MaxFTSQueryLen {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("la consulta de búsqueda excede la longitud máxima (%d caracteres).", MaxFTSQueryLen),
			Cause:   fmt.Errorf("la consulta excede la longitud máxima: %w", domain.ErrInvalidInput),
		}
	}
	if ftsQueryRe.MatchString(query) || ftsTermBoundaryRe.MatchString(query) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la consulta de búsqueda contiene caracteres no permitidos.",
			Cause:   fmt.Errorf("la consulta contiene caracteres no permitidos: %w", domain.ErrInvalidInput),
		}
	}
	if isFTSOperatorQuery(query) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la consulta de búsqueda contiene operadores no permitidos.",
			Cause:   fmt.Errorf("la consulta contiene operadores FTS5 no permitidos: %w", domain.ErrInvalidInput),
		}
	}
	return nil
}

// isFTSOperatorQuery rejects whole-word FTS5 operators and a leading '-'.
// The leading '-' branch is a defensive guard: term-boundary hyphens are
// already rejected by validateFTSQuery's ftsTermBoundaryRe check, but it stays
// in case the check order changes. Inline hyphens (e.g. "geo-local") remain
// valid because the tokenizer keeps the term as one token.
func isFTSOperatorQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if len(trimmed) > 0 && trimmed[0] == '-' {
		return true
	}
	return ftsOperatorRe.MatchString(query)
}

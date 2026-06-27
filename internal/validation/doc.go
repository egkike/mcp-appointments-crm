// Package validation provides reusable input-format validators for the project.
//
// This package is responsible for input-format validation only:
//   - ValidateEmail(s string) error
//   - ValidatePhone(s string) error
//   - ValidateTime(s, format string) error
//   - ValidateLatitude(f float64) error
//   - ValidateLongitude(f float64) error
//   - etc.
//
// Business-domain semantic errors (e.g., "el profesional no trabaja los domingos")
// live in internal/repository/errors.go as SemanticError{Code, Message, Cause}.
// This package does NOT own those error types.
//
// Consumers of this package:
//   - The config-wizard TUI (internal/tui) to validate user input field-by-field
//   - The MCP handlers (internal/mcp) to validate tool arguments from Hermes
//
// This is a leaf package — it has no dependency on internal/repository.
// For the full error contract (sentinels + SemanticError), see the data-access spec.
//
// Error messages are written in neutral Spanish to match the project's
// documentation language and to provide clear, actionable feedback to end
// users. Stack traces are NEVER included in the message sent to the LLM.
// Server-side logs may include the full wrapped error via
// fmt.Errorf("...: %w", err).
package validation

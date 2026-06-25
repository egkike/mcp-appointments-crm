// Package validation provides reusable input validators AND the project's
// standard error codes/types for validation failures.
//
// Two responsibilities, one package, because they are tightly coupled:
//
//  1. Validators: functions that take a value and return an error, e.g.:
//     - ValidateEmail(s string) error
//     - ValidatePhone(s string) error
//     - ValidateTime(s, format string) error
//     - ValidateLatitude(f float64) error
//     - ValidateLongitude(f float64) error
//     - etc.
//
//  2. Error codes and types: constants and structs that describe what went
//     wrong, in semantic Spanish. All errors returned to the LLM (Hermes)
//     via the MCP protocol must use the types defined here.
//     - ErrCodeInvalidEmail, ErrCodeInvalidTime, etc. (typed string constants)
//     - type Error struct { Code ErrCode; Message string; Cause error }
//
// Consumers of this package:
//   - The config-wizard TUI (internal/tui) to validate user input field-by-field
//   - The MCP handlers (internal/mcp) to validate tool arguments from Hermes
//   - Any future business-logic layer for domain validation
//
// Error messages are written in neutral Spanish to match the project's
// documentation language and to provide clear, actionable feedback to end
// users. Stack traces are NEVER included in the message sent to the LLM.
// Server-side logs may include the full wrapped error via
// fmt.Errorf("...: %w", err).
package validation

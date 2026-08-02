// Package repository provides the data-access layer for the application.
//
// Error handling contract:
//   - Sentinel errors (ErrNotFound, ErrConflict, ErrInvalidInput) for CRUD
//     control flow, usable with errors.Is.
//   - SemanticError for business-domain errors (e.g., the 5-step
//     check_availability chain), usable with errors.As.
//
// NOTE: Error types are aliased to internal/domain during the clean-architecture
// migration (P3.1). These aliases will be removed in P3.4b when all consumers
// import domain types directly.
package repository

import (
	"errors"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"modernc.org/sqlite"
)

// ─── Error type aliases (P3.1b — migration to domain layer) ────────────────
// These aliases allow existing repository code to compile unchanged while the
// canonical definitions live in internal/domain/errors.go.

// SemanticError is an alias for domain.SemanticError.
type SemanticError = domain.SemanticError

// ErrCode is an alias for domain.ErrCode.
type ErrCode = domain.ErrCode

// Sentinel errors for CRUD-level conditions (aliased to domain).
var (
	ErrNotFound     = domain.ErrNotFound
	ErrConflict     = domain.ErrConflict
	ErrInvalidInput = domain.ErrInvalidInput
)

// Error code constants (aliased to domain).
const (
	ErrCodeBusinessClosed         = domain.ErrCodeBusinessClosed
	ErrCodeProfessionalNotWorking = domain.ErrCodeProfessionalNotWorking
	ErrCodeServiceNotActive       = domain.ErrCodeServiceNotActive
	ErrCodeProfessionalNotActive  = domain.ErrCodeProfessionalNotActive
	ErrCodeSlotOutOfHours         = domain.ErrCodeSlotOutOfHours
	ErrCodeBookingOverlap         = domain.ErrCodeBookingOverlap
	ErrCodeSlotInPast             = domain.ErrCodeSlotInPast
	ErrCodeNotFound               = domain.ErrCodeNotFound
	ErrCodeConflict               = domain.ErrCodeConflict
	ErrCodeInvalidInput           = domain.ErrCodeInvalidInput
	ErrCodeInternal               = domain.ErrCodeInternal
	ErrCodeUnauthenticated        = domain.ErrCodeUnauthenticated
)

// sqliteConstraintUnique is the SQLite extended result code for
// SQLITE_CONSTRAINT_UNIQUE.
const sqliteConstraintUnique = 2067

// isUniqueViolation checks whether err is a SQLite UNIQUE constraint error.
// Primary path: typed check via *sqlite.Error.Code() for reliability.
// Fallback: string match for drivers that don't expose *sqlite.Error
// (e.g., go-sqlmock in tests).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqliteConstraintUnique
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

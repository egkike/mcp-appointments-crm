// Package repository provides the data-access layer for the application.
//
// Error handling contract:
//   - Sentinel errors (domain.ErrNotFound, domain.ErrConflict, domain.ErrInvalidInput)
//     for CRUD control flow, usable with errors.Is.
//   - domain.SemanticError for business-domain errors (e.g., the 5-step
//     check_availability chain), usable with errors.As.
package repository

import (
	"errors"
	"strings"

	"modernc.org/sqlite"
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

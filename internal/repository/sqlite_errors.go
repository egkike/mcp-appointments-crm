package repository

import (
	"errors"
	"strings"

	"modernc.org/sqlite"
)

// SQLite extended result codes that mean "duplicate key" for this schema.
// accounts.id is TEXT PRIMARY KEY and clients.phone is UNIQUE, so a duplicate
// insert surfaces as SQLITE_CONSTRAINT_PRIMARYKEY (1555) for accounts.id or
// SQLITE_CONSTRAINT_UNIQUE (2067) for clients.phone. Both codes mean "id/phone
// already exists" and map to domain.ErrConflict.
const (
	sqliteConstraintUnique     = 2067
	sqliteConstraintPrimaryKey = 1555
)

// isUniqueViolation checks whether err is a duplicate-key constraint error
// (UNIQUE or PRIMARY KEY).
// Primary path: typed check via *sqlite.Error.Code() for reliability.
// Fallback: string match for drivers that don't expose *sqlite.Error
// (e.g., go-sqlmock in tests).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		return code == sqliteConstraintUnique || code == sqliteConstraintPrimaryKey
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "PRIMARY KEY constraint failed")
}

// isSingleOwnerViolation checks if the error is the SQLite single-owner trigger.
func isSingleOwnerViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "single-owner invariant")
}

package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
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

// sqliteConstraintForeignKey is SQLITE_CONSTRAINT_FOREIGNKEY (787). SQLite
// raises it for a dangling reference: an INSERT/UPDATE that points at a parent
// row which does not exist (e.g. schedules.professional_id) or a parent delete
// blocked by a child declared ON DELETE NO ACTION.
const sqliteConstraintForeignKey = 787

// sqliteConstraintTrigger is SQLITE_CONSTRAINT_TRIGGER (1811). SQLite reports a
// parent delete blocked by an ON DELETE RESTRICT child under this code instead
// of 787 (SQLite enforces RESTRICT through an implicit trigger). This is the
// shape the actual schema produces: bookings.service_id and
// bookings.professional_id are ON DELETE RESTRICT in schema.go, so deleting a
// service that still has bookings raises 1811. Because the accounts
// single-owner triggers also RAISE(ABORT) → 1811, this code is only treated as
// a foreign-key failure when the message confirms it (see
// foreignKeyViolationMessage).
const sqliteConstraintTrigger = 1811

// foreignKeyViolationMessage is the SQLite message shared by both codes above.
// Only the message distinguishes "FOREIGN KEY constraint failed" from an
// application trigger abort that uses the same result code.
const foreignKeyViolationMessage = "FOREIGN KEY constraint failed"

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

// isForeignKeyViolation checks whether err is a foreign-key enforcement failure:
// either SQLITE_CONSTRAINT_FOREIGNKEY (787, dangling reference) or
// SQLITE_CONSTRAINT_TRIGGER (1811, parent delete blocked by an ON DELETE
// RESTRICT child). The 1811 case requires the message to match, so the
// single-owner triggers are not misread as foreign-key errors.
// Same two-step strategy as isUniqueViolation: typed code check for reliability,
// string fallback for drivers that don't expose *sqlite.Error (e.g., go-sqlmock
// in tests).
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case sqliteConstraintForeignKey:
			return true
		case sqliteConstraintTrigger:
			return strings.Contains(err.Error(), foreignKeyViolationMessage)
		default:
			return false
		}
	}
	return strings.Contains(err.Error(), foreignKeyViolationMessage)
}

// classifyForeignKeyViolation wraps a foreign-key violation around
// domain.ErrConflict, so callers branch with errors.Is instead of inspecting
// SQLite result codes (same shape as the uniqueness mappings in clients.go and
// accounts.go). Non-foreign-key errors are returned unchanged, so this is a safe
// pass-through in repository error paths.
func classifyForeignKeyViolation(err error) error {
	if !isForeignKeyViolation(err) {
		return err
	}
	return fmt.Errorf("foreign key constraint violation: %w", domain.ErrConflict)
}

// isSingleOwnerViolation checks if the error is the SQLite single-owner trigger.
func isSingleOwnerViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "single-owner invariant")
}

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

// applicationTriggerMessages is the single source of truth for the RAISE(ABORT)
// application triggers whose failures share SQLITE_CONSTRAINT_TRIGGER (1811) with
// SQLite's implicit ON DELETE RESTRICT foreign-key trigger. Each entry is a
// distinctive marker of the full RAISE(ABORT) message and is matched with
// strings.Contains, so a stable prefix is enough.
//
// Maintenance contract: any new application trigger in internal/db/schema.go
// that RAISE(ABORT)s MUST add its message marker here AND get a classifier test
// in sqlite_errors_test.go. Without that registration a same-code abort whose
// message happens to contain "FOREIGN KEY constraint failed" could be
// misclassified as a foreign-key conflict and mapped to domain.ErrConflict.
var applicationTriggerMessages = []string{
	"single-owner invariant",
}

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

// matchesApplicationTriggerMessage reports whether msg was raised by a trigger
// registered in applicationTriggerMessages (see its maintenance contract).
func matchesApplicationTriggerMessage(msg string) bool {
	for _, marker := range applicationTriggerMessages {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// isForeignKeyViolation checks whether err is a foreign-key enforcement failure:
// either SQLITE_CONSTRAINT_FOREIGNKEY (787, dangling reference) or
// SQLITE_CONSTRAINT_TRIGGER (1811, parent delete blocked by an ON DELETE
// RESTRICT child).
//
// The 1811 code is guarded on BOTH sides: it counts as a foreign-key failure
// only when the message contains foreignKeyViolationMessage AND no registered
// application-trigger marker matches. The exclusion is defense-in-depth against
// a future RAISE(ABORT) trigger whose message embeds the FK string; the
// fail-safe direction is deliberate — an unknown 1811 message stays
// unclassified rather than being forced into domain.ErrConflict.
//
// Same two-step strategy as isUniqueViolation: typed code check for reliability,
// string fallback for drivers that don't expose *sqlite.Error (e.g., go-sqlmock
// in tests). The two-sided guard applies to both paths.
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
			return isForeignKeyMessage(err.Error())
		default:
			return false
		}
	}
	return isForeignKeyMessage(err.Error())
}

// isForeignKeyMessage applies the two-sided guard for the message-only paths:
// the SQLite FK message must be present and no application trigger may own the
// message.
func isForeignKeyMessage(msg string) bool {
	return strings.Contains(msg, foreignKeyViolationMessage) &&
		!matchesApplicationTriggerMessage(msg)
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
// It resolves the trigger message through applicationTriggerMessages so the
// registry stays the single source of truth for 1811 application-trigger aborts.
func isSingleOwnerViolation(err error) bool {
	if err == nil {
		return false
	}
	return matchesApplicationTriggerMessage(err.Error())
}

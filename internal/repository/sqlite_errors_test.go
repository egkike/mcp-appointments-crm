package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"modernc.org/sqlite"
)

func TestSentinelErrors_ErrorsIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
		is   error
	}{
		{"domain.ErrNotFound direct", domain.ErrNotFound, domain.ErrNotFound},
		{"domain.ErrConflict direct", domain.ErrConflict, domain.ErrConflict},
		{"domain.ErrInvalidInput direct", domain.ErrInvalidInput, domain.ErrInvalidInput},
		{"domain.ErrNotFound wrapped", fmt.Errorf("get client: %w", domain.ErrNotFound), domain.ErrNotFound},
		{"domain.ErrConflict wrapped", fmt.Errorf("create service: %w", domain.ErrConflict), domain.ErrConflict},
		{"domain.ErrInvalidInput wrapped", fmt.Errorf("validate input: %w", domain.ErrInvalidInput), domain.ErrInvalidInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false; want true", tt.err, tt.is)
			}
		})
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{domain.ErrNotFound, domain.ErrConflict, domain.ErrInvalidInput}
	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("sentinel %v should not match %v", sentinels[i], sentinels[j])
			}
		}
	}
}

func TestSemanticError_Error(t *testing.T) {
	e := &domain.SemanticError{
		Code:    domain.ErrCodeBusinessClosed,
		Message: "el negocio está cerrado el 2026-12-25 (Navidad)",
	}
	if got := e.Error(); got != e.Message {
		t.Errorf("Error() = %q; want %q", got, e.Message)
	}
}

func TestSemanticError_Unwrap(t *testing.T) {
	cause := errors.New("database timeout")
	e := &domain.SemanticError{
		Code:    domain.ErrCodeInternal,
		Message: "error interno",
		Cause:   cause,
	}
	unwrapped := errors.Unwrap(e)
	if unwrapped != cause { //nolint:errorlint // test verifies Unwrap returns the exact cause pointer
		t.Errorf("Unwrap() should return cause; got %v (%T), want %v (%T)", unwrapped, unwrapped, cause, cause)
	}
}

func TestSemanticError_ErrorsAs(t *testing.T) {
	original := &domain.SemanticError{
		Code:    domain.ErrCodeBookingOverlap,
		Message: "el Profesional Juan ya tiene una reserva de 10:00 a 11:00.",
	}
	wrapped := fmt.Errorf("create booking: %w", original)

	var sErr *domain.SemanticError
	if !errors.As(wrapped, &sErr) {
		t.Fatal("errors.As should extract *domain.SemanticError from wrapped error")
	}
	if sErr.Code != domain.ErrCodeBookingOverlap {
		t.Errorf("Code = %q; want %q", sErr.Code, domain.ErrCodeBookingOverlap)
	}
	if sErr.Message != original.Message {
		t.Errorf("Message = %q; want %q", sErr.Message, original.Message)
	}
}

func TestSemanticError_NilCause(t *testing.T) {
	e := &domain.SemanticError{
		Code:    domain.ErrCodeSlotInPast,
		Message: "no se puede reservar en el pasado.",
	}
	if e.Unwrap() != nil {
		t.Error("Unwrap() should return nil when Cause is nil")
	}
}

func TestErrCode_Constants(t *testing.T) {
	codes := []domain.ErrCode{
		domain.ErrCodeBusinessClosed,
		domain.ErrCodeProfessionalNotWorking,
		domain.ErrCodeSlotOutOfHours,
		domain.ErrCodeBookingOverlap,
		domain.ErrCodeSlotInPast,
		domain.ErrCodeNotFound,
		domain.ErrCodeConflict,
		domain.ErrCodeInvalidInput,
		domain.ErrCodeInternal,
	}
	for _, code := range codes {
		if string(code) == "" {
			t.Errorf("domain.ErrCode constant is empty")
		}
	}
}

func TestIsUniqueViolation(t *testing.T) {
	// Trigger a real *sqlite.Error via an in-memory SQLite UNIQUE violation.
	// sqlite.Error has unexported fields, so we can't construct it directly.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT UNIQUE)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO t (v) VALUES ('a')"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, uniqueErr := db.ExecContext(ctx, "INSERT INTO t (v) VALUES ('a')")
	if uniqueErr == nil {
		t.Fatal("expected UNIQUE violation error, got nil")
	}

	// Trigger a real *sqlite.Error with SQLITE_CONSTRAINT_PRIMARYKEY (1555)
	// via a duplicate TEXT PRIMARY KEY, mirroring accounts.id.
	if _, err := db.ExecContext(ctx, "CREATE TABLE pk (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create pk table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO pk (id) VALUES ('dup')"); err != nil {
		t.Fatalf("insert pk: %v", err)
	}
	_, primaryKeyErr := db.ExecContext(ctx, "INSERT INTO pk (id) VALUES ('dup')")
	if primaryKeyErr == nil {
		t.Fatal("expected PRIMARY KEY violation error, got nil")
	}
	// Guard the driver contract: if SQLite stops reporting code 1555 for a TEXT
	// PRIMARY KEY duplicate, this case must fail loudly instead of silently
	// re-covering the 2067 path.
	if !strings.Contains(primaryKeyErr.Error(), "(1555)") {
		t.Fatalf("expected SQLITE_CONSTRAINT_PRIMARYKEY (1555), got %v", primaryKeyErr)
	}

	// Build a non-UNIQUE *sqlite.Error by dropping a non-existent table.
	_, nonUniqueErr := db.ExecContext(ctx, "DROP TABLE nonexistent")
	if nonUniqueErr == nil {
		t.Fatal("expected error for DROP TABLE nonexistent, got nil")
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"plain UNIQUE string match", errors.New("UNIQUE constraint failed: clients.phone"), true},
		{"plain non-UNIQUE error", errors.New("disk I/O error"), false},
		{"wrapped UNIQUE string match", fmt.Errorf("insert: %w", errors.New("UNIQUE constraint failed: x")), true},
		{"empty error message", errors.New(""), false},
		{"typed *sqlite.Error UNIQUE (code 2067)", uniqueErr, true},
		{"wrapped typed *sqlite.Error UNIQUE", fmt.Errorf("insert: %w", uniqueErr), true},
		{"typed *sqlite.Error PRIMARYKEY (code 1555)", primaryKeyErr, true},
		{"wrapped typed *sqlite.Error PRIMARYKEY", fmt.Errorf("insert: %w", primaryKeyErr), true},
		{"plain PRIMARY KEY string match", errors.New("PRIMARY KEY constraint failed: accounts.id"), true},
		{"foreign key error is not a unique violation", errors.New("FOREIGN KEY constraint failed"), false},
		{"typed *sqlite.Error non-UNIQUE", nonUniqueErr, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUniqueViolation(tt.err)
			if got != tt.want {
				t.Errorf("isUniqueViolation(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}

// foreignKeyViolationErrors returns the real *sqlite.Error shapes a foreign-key
// failure takes in this schema:
//   - dangling: UPDATE points at a parent row that does not exist →
//     SQLITE_CONSTRAINT_FOREIGNKEY (787). Mirrors
//     schedules.professional_id on the upsert_schedule path.
//   - restricted: DELETE of a parent still referenced by an ON DELETE RESTRICT
//     child → SQLITE_CONSTRAINT_TRIGGER (1811). Mirrors bookings.service_id
//     (schema.go), the delete_service path.
//   - triggerAbort: a custom RAISE(ABORT) → also 1811, but NOT a foreign-key
//     failure. Mirrors the accounts single-owner triggers.
func foreignKeyViolationErrors(t *testing.T) (dangling, restricted, triggerAbort error) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// A single connection keeps the in-memory schema and the per-connection
	// PRAGMA foreign_keys visible to every statement of this test.
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	for _, stmt := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE parent (id TEXT PRIMARY KEY)",
		"CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id) ON DELETE RESTRICT)",
		"INSERT INTO parent (id) VALUES ('parent-1')",
		"INSERT INTO child (id, parent_id) VALUES ('child-1', 'parent-1')",
		"CREATE TABLE guarded (id TEXT PRIMARY KEY, flag INTEGER NOT NULL DEFAULT 0)",
		"CREATE TRIGGER guarded_abort BEFORE INSERT ON guarded WHEN NEW.flag = 1 BEGIN SELECT RAISE(ABORT, 'custom trigger abort'); END",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}

	_, dangling = db.ExecContext(ctx, "UPDATE child SET parent_id = 'ghost' WHERE id = 'child-1'")
	if dangling == nil {
		t.Fatal("expected FOREIGN KEY violation for a dangling reference, got nil")
	}
	_, restricted = db.ExecContext(ctx, "DELETE FROM parent WHERE id = 'parent-1'")
	if restricted == nil {
		t.Fatal("expected FOREIGN KEY violation deleting a restricted parent, got nil")
	}
	_, triggerAbort = db.ExecContext(ctx, "INSERT INTO guarded (id, flag) VALUES ('g1', 1)")
	if triggerAbort == nil {
		t.Fatal("expected trigger abort, got nil")
	}
	return dangling, restricted, triggerAbort
}

// sqliteCode extracts the extended result code reported by the driver.
func sqliteCode(t *testing.T, err error) int {
	t.Helper()
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("expected *sqlite.Error, got %T: %v", err, err)
	}
	return sqliteErr.Code()
}

func TestIsForeignKeyViolation(t *testing.T) {
	dangling, restricted, triggerAbort := foreignKeyViolationErrors(t)

	// Guard the driver contract: if modernc.org/sqlite changes the result code
	// of either FK shape, fail loudly instead of silently falling back to the
	// message match.
	if got := sqliteCode(t, dangling); got != sqliteConstraintForeignKey {
		t.Fatalf("dangling reference code = %d, want %d (%v)", got, sqliteConstraintForeignKey, dangling)
	}
	if got := sqliteCode(t, restricted); got != sqliteConstraintTrigger {
		t.Fatalf("RESTRICT parent delete code = %d, want %d (%v)", got, sqliteConstraintTrigger, restricted)
	}
	if got := sqliteCode(t, triggerAbort); got != sqliteConstraintTrigger {
		t.Fatalf("custom trigger abort code = %d, want %d (%v)", got, sqliteConstraintTrigger, triggerAbort)
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"typed *sqlite.Error dangling reference (787)", dangling, true},
		{"wrapped typed *sqlite.Error dangling reference", fmt.Errorf("upsert horario: %w", dangling), true},
		{"typed *sqlite.Error RESTRICT parent delete (1811)", restricted, true},
		{"wrapped typed *sqlite.Error RESTRICT parent delete", fmt.Errorf("eliminar servicio: %w", restricted), true},
		{"typed *sqlite.Error custom trigger abort (1811) is not a FK failure", triggerAbort, false},
		{"plain FOREIGN KEY string match", errors.New("FOREIGN KEY constraint failed"), true},
		{"wrapped FOREIGN KEY string match", fmt.Errorf("upsert horario: %w", errors.New("FOREIGN KEY constraint failed")), true},
		{"UNIQUE is not a foreign key violation", errors.New("UNIQUE constraint failed: clients.phone"), false},
		{"PRIMARY KEY is not a foreign key violation", errors.New("PRIMARY KEY constraint failed: accounts.id"), false},
		{"plain non-constraint error", errors.New("disk I/O error"), false},
		{"empty error message", errors.New(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isForeignKeyViolation(tt.err); got != tt.want {
				t.Errorf("isForeignKeyViolation(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestClassifyForeignKeyViolation(t *testing.T) {
	dangling, restricted, triggerAbort := foreignKeyViolationErrors(t)

	fkCases := []struct {
		name string
		err  error
	}{
		{"dangling reference (787)", dangling},
		{"RESTRICT parent delete (1811)", restricted},
	}
	for _, tc := range fkCases {
		t.Run(tc.name+" maps to domain.ErrConflict", func(t *testing.T) {
			// Same shape the repository call sites produce: call-site context
			// around the raw driver error.
			classified := classifyForeignKeyViolation(fmt.Errorf("eliminar servicio: %w", tc.err))
			if !errors.Is(classified, domain.ErrConflict) {
				t.Fatalf("errors.Is(%v, domain.ErrConflict) = false; want true", classified)
			}
		})
	}

	t.Run("string fallback FK violation maps to domain.ErrConflict", func(t *testing.T) {
		classified := classifyForeignKeyViolation(errors.New("FOREIGN KEY constraint failed"))
		if !errors.Is(classified, domain.ErrConflict) {
			t.Fatalf("errors.Is(%v, domain.ErrConflict) = false; want true", classified)
		}
	})

	t.Run("custom trigger abort passes through unchanged", func(t *testing.T) {
		classified := classifyForeignKeyViolation(triggerAbort)
		if !errors.Is(classified, triggerAbort) {
			t.Errorf("non-FK trigger abort must pass through, got %v", classified)
		}
		if errors.Is(classified, domain.ErrConflict) {
			t.Errorf("custom trigger abort must not be classified as a foreign key conflict: %v", classified)
		}
	})

	t.Run("UNIQUE violation passes through unchanged", func(t *testing.T) {
		unique := errors.New("UNIQUE constraint failed: clients.phone")
		classified := classifyForeignKeyViolation(unique)
		if !errors.Is(classified, unique) {
			t.Errorf("non-FK error must pass through, got %v", classified)
		}
		if errors.Is(classified, domain.ErrConflict) {
			t.Errorf("UNIQUE violation must not be classified as a foreign key conflict: %v", classified)
		}
	})

	t.Run("nil error stays nil", func(t *testing.T) {
		if classified := classifyForeignKeyViolation(nil); classified != nil {
			t.Errorf("classifyForeignKeyViolation(nil) = %v; want nil", classified)
		}
	})
}

// TestRepositoryDoesNotImportValidation verifies that the repository package
// does NOT import internal/validation (per design Decisión 5 and ADR-0005).
// This is a structural assertion to prevent circular dependencies.
func TestRepositoryDoesNotImportValidation(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.Imports}}", "github.com/egkike/mcp-appointments-crm/internal/repository")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports: %v\n%s", err, out)
	}
	imports := string(out)
	if strings.Contains(imports, "internal/validation") {
		t.Errorf("repository package must NOT import internal/validation (ADR-0005); found in: %s", imports)
	}
}

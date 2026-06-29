package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSentinelErrors_ErrorsIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
		is   error
	}{
		{"ErrNotFound direct", ErrNotFound, ErrNotFound},
		{"ErrConflict direct", ErrConflict, ErrConflict},
		{"ErrInvalidInput direct", ErrInvalidInput, ErrInvalidInput},
		{"ErrNotFound wrapped", fmt.Errorf("get client: %w", ErrNotFound), ErrNotFound},
		{"ErrConflict wrapped", fmt.Errorf("create service: %w", ErrConflict), ErrConflict},
		{"ErrInvalidInput wrapped", fmt.Errorf("validate input: %w", ErrInvalidInput), ErrInvalidInput},
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
	sentinels := []error{ErrNotFound, ErrConflict, ErrInvalidInput}
	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("sentinel %v should not match %v", sentinels[i], sentinels[j])
			}
		}
	}
}

func TestSemanticError_Error(t *testing.T) {
	e := &SemanticError{
		Code:    ErrCodeBusinessClosed,
		Message: "el negocio está cerrado el 2026-12-25 (Navidad)",
	}
	if got := e.Error(); got != e.Message {
		t.Errorf("Error() = %q; want %q", got, e.Message)
	}
}

func TestSemanticError_Unwrap(t *testing.T) {
	cause := errors.New("database timeout")
	e := &SemanticError{
		Code:    ErrCodeInternal,
		Message: "error interno",
		Cause:   cause,
	}
	unwrapped := errors.Unwrap(e)
	if !errors.Is(unwrapped, cause) {
		t.Errorf("Unwrap() should expose cause; got %v (%T), want %v (%T)", unwrapped, unwrapped, cause, cause)
	}
}

func TestSemanticError_ErrorsAs(t *testing.T) {
	original := &SemanticError{
		Code:    ErrCodeBookingOverlap,
		Message: "el Profesional Juan ya tiene una reserva de 10:00 a 11:00.",
	}
	wrapped := fmt.Errorf("create booking: %w", original)

	var sErr *SemanticError
	if !errors.As(wrapped, &sErr) {
		t.Fatal("errors.As should extract *SemanticError from wrapped error")
	}
	if sErr.Code != ErrCodeBookingOverlap {
		t.Errorf("Code = %q; want %q", sErr.Code, ErrCodeBookingOverlap)
	}
	if sErr.Message != original.Message {
		t.Errorf("Message = %q; want %q", sErr.Message, original.Message)
	}
}

func TestSemanticError_NilCause(t *testing.T) {
	e := &SemanticError{
		Code:    ErrCodeSlotInPast,
		Message: "no se puede reservar en el pasado.",
	}
	if e.Unwrap() != nil {
		t.Error("Unwrap() should return nil when Cause is nil")
	}
}

func TestErrCode_Constants(t *testing.T) {
	codes := []ErrCode{
		ErrCodeBusinessClosed,
		ErrCodeProfessionalNotWorking,
		ErrCodeSlotOutOfHours,
		ErrCodeBookingOverlap,
		ErrCodeSlotInPast,
		ErrCodeNotFound,
		ErrCodeConflict,
		ErrCodeInvalidInput,
		ErrCodeInternal,
	}
	for _, code := range codes {
		if string(code) == "" {
			t.Errorf("ErrCode constant is empty")
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

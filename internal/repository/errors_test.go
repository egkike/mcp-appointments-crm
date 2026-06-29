package repository

import (
	"errors"
	"fmt"
	"testing"
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
	if !errors.Is(e, cause) {
		t.Error("Unwrap should expose the cause")
	}
	unwrapped := errors.Unwrap(e)
	if !errors.Is(e, cause) {
		t.Errorf("Unwrap() should expose cause; got %v, want %v", unwrapped, cause)
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

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestSemanticError_Error(t *testing.T) {
	e := &SemanticError{
		Code:    ErrCodeBookingOverlap,
		Message: "el horario solicitado se superpone con otra reserva",
	}
	want := "el horario solicitado se superpone con otra reserva"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSemanticError_Unwrap(t *testing.T) {
	cause := errors.New("underlying cause")
	e := &SemanticError{
		Code:    ErrCodeInternal,
		Message: "error interno",
		Cause:   cause,
	}
	if !errors.Is(e.Unwrap(), cause) {
		t.Errorf("Unwrap() = %v, want %v", e.Unwrap(), cause)
	}

	// errors.Is should traverse the cause chain
	wrapped := &SemanticError{
		Code:    ErrCodeInternal,
		Message: "wrapped",
		Cause:   ErrNotFound,
	}
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("errors.Is(wrapped, ErrNotFound) = false, want true")
	}
}

func TestSemanticError_NilCause(t *testing.T) {
	e := &SemanticError{
		Code:    ErrCodeSlotInPast,
		Message: "el horario es en el pasado",
	}
	if got := e.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestErrorCodes_AreDistinct(t *testing.T) {
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
		ErrCodeUnauthenticated,
	}
	seen := make(map[ErrCode]bool, len(codes))
	for i, c := range codes {
		if seen[c] {
			t.Errorf("duplicate error code %q at index %d", c, i)
		}
		seen[c] = true
	}
}

func TestErrorCodes_MatchExpectedStrings(t *testing.T) {
	tests := []struct {
		code ErrCode
		want string
	}{
		{ErrCodeBusinessClosed, "BUSINESS_CLOSED"},
		{ErrCodeProfessionalNotWorking, "PROFESSIONAL_NOT_WORKING"},
		{ErrCodeSlotOutOfHours, "SLOT_OUT_OF_HOURS"},
		{ErrCodeBookingOverlap, "BOOKING_OVERLAP"},
		{ErrCodeSlotInPast, "SLOT_IN_PAST"},
		{ErrCodeNotFound, "NOT_FOUND"},
		{ErrCodeConflict, "CONFLICT"},
		{ErrCodeInvalidInput, "INVALID_INPUT"},
		{ErrCodeInternal, "INTERNAL"},
		{ErrCodeUnauthenticated, "UNAUTHENTICATED"},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if string(tt.code) != tt.want {
				t.Errorf("ErrCode = %q, want %q", string(tt.code), tt.want)
			}
		})
	}
}

func TestSentinelErrors_WorkWithErrorsIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
		is   error
		want bool
	}{
		{"ErrNotFound matches itself", ErrNotFound, ErrNotFound, true},
		{"ErrConflict matches itself", ErrConflict, ErrConflict, true},
		{"ErrInvalidInput matches itself", ErrInvalidInput, ErrInvalidInput, true},
		{"ErrUnauthenticated matches itself", ErrUnauthenticated, ErrUnauthenticated, true},
		{"ErrNotFound does not match ErrConflict", ErrNotFound, ErrConflict, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.is); got != tt.want {
				t.Errorf("errors.Is() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrUnauthenticated_CanonicalMessage(t *testing.T) {
	// The canonical message must be "caller not authenticated"
	// (consolidated from repo's "caller not authenticated" and auth's "unauthenticated")
	msg := ErrUnauthenticated.Error()
	if !strings.Contains(msg, "caller not authenticated") {
		t.Errorf("ErrUnauthenticated.Error() = %q; must contain %q", msg, "caller not authenticated")
	}
}

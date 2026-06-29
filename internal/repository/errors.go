// Package repository provides the data-access layer for the application.
//
// Error handling contract:
//   - Sentinel errors (ErrNotFound, ErrConflict, ErrInvalidInput) for CRUD
//     control flow, usable with errors.Is.
//   - SemanticError for business-domain errors (e.g., the 5-step
//     check_availability chain), usable with errors.As.
//
// This package does NOT import internal/validation.
package repository

import "errors"

// Sentinel errors for CRUD-level conditions.
var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrConflict indicates a uniqueness or foreign-key constraint was violated.
	ErrConflict = errors.New("constraint violation")

	// ErrInvalidInput indicates the input failed application-level validation.
	ErrInvalidInput = errors.New("invalid input")
)

// ErrCode identifies the category of a business-domain error.
type ErrCode string

const (
	ErrCodeBusinessClosed         ErrCode = "BUSINESS_CLOSED"
	ErrCodeProfessionalNotWorking ErrCode = "PROFESSIONAL_NOT_WORKING"
	ErrCodeSlotOutOfHours         ErrCode = "SLOT_OUT_OF_HOURS"
	ErrCodeBookingOverlap         ErrCode = "BOOKING_OVERLAP"
	ErrCodeSlotInPast             ErrCode = "SLOT_IN_PAST"
	ErrCodeNotFound               ErrCode = "NOT_FOUND"
	ErrCodeConflict               ErrCode = "CONFLICT"
	ErrCodeInvalidInput           ErrCode = "INVALID_INPUT"
	ErrCodeInternal               ErrCode = "INTERNAL"
)

// SemanticError represents a business-domain error with a machine-readable
// code, a human-readable Spanish message, and an optional cause for
// server-side logging.
type SemanticError struct {
	Code    ErrCode
	Message string
	Cause   error
}

// Error returns the human-readable message.
func (e *SemanticError) Error() string { return e.Message }

// Unwrap returns the underlying cause, if any.
func (e *SemanticError) Unwrap() error { return e.Cause }

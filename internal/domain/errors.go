// Package domain contains the domain layer: entities, repository interfaces,
// domain services, and domain errors.
//
// Zero dependencies rule: this package MUST NOT import any package outside
// itself. No imports of database/sql, internal/repository/, internal/auth/,
// internal/db/, net/http, or any external transport library.
package domain

import "errors"

// ErrCode identifies the category of a business-domain error.
type ErrCode string

const (
	ErrCodeBusinessClosed         ErrCode = "BUSINESS_CLOSED"
	ErrCodeProfessionalNotWorking ErrCode = "PROFESSIONAL_NOT_WORKING"
	ErrCodeServiceNotActive       ErrCode = "SERVICE_NOT_ACTIVE"
	ErrCodeProfessionalNotActive  ErrCode = "PROFESSIONAL_NOT_ACTIVE"
	ErrCodeSlotOutOfHours         ErrCode = "SLOT_OUT_OF_HOURS"
	ErrCodeBookingOverlap         ErrCode = "BOOKING_OVERLAP"
	ErrCodeSlotInPast             ErrCode = "SLOT_IN_PAST"
	ErrCodeNotFound               ErrCode = "NOT_FOUND"
	ErrCodeConflict               ErrCode = "CONFLICT"
	ErrCodeInvalidInput           ErrCode = "INVALID_INPUT"
	ErrCodeInternal               ErrCode = "INTERNAL"
	// ErrCodeUnauthenticated covers both missing authentication (401) and
	// insufficient authorization (403-ish). The preferred approach is
	// dynamic-WHERE authorization: the query itself filters by caller scope,
	// so cross-tenant and non-existent rows both return ErrNotFound.
	ErrCodeUnauthenticated ErrCode = "UNAUTHENTICATED"
)

// Sentinel errors for CRUD-level conditions.
var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrConflict indicates a uniqueness or foreign-key constraint was violated.
	ErrConflict = errors.New("constraint violation")

	// ErrInvalidInput indicates the input failed application-level validation.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthenticated is the canonical sentinel for authentication failures.
	// Consolidated from repository/auth_helpers.go ("caller not authenticated")
	// and auth/resolver.go ("unauthenticated"). The canonical message is
	// "caller not authenticated".
	ErrUnauthenticated = errors.New("caller not authenticated")
)

// SemanticError represents a business-domain error with a machine-readable
// code, a human-readable message, and an optional cause for server-side logging.
type SemanticError struct {
	Code    ErrCode
	Message string
	Cause   error
}

// Error returns the human-readable message.
func (e *SemanticError) Error() string { return e.Message }

// Unwrap returns the underlying cause, if any.
func (e *SemanticError) Unwrap() error { return e.Cause }

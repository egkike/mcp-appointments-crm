package service

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// BookingValidator is a stateless domain service that orchestrates
// booking-datetime validation for booking mutations. It delegates the 5-step
// chain to the shared ValidateBookingTimeSlot helper and returns the first
// *domain.SemanticError encountered, or nil on success (REQ-BV-1, REQ-BV-3).
//
// It holds no *sql.DB, no SQL connections, and no mutable state, so it is safe
// to share as a singleton across use cases (REQ-BV-5, REQ-BV-6).
//
// Interface placement (PR #A scope, deferred to PR #B):
//
// The consumer-facing interface `domain.BookingValidator` (with signature
// `Validate(ctx context.Context, input ValidateBookingInput) *domain.SemanticError`)
// is NOT declared in this package. Per the `internal/domain/` zero-dependency
// rule (see `internal/domain/errors.go`), the domain package MUST NOT import
// `internal/domain/entity/` — but `ValidateBookingInput` references entity
// types, so the interface cannot live here. It will be declared in PR #B in
// the use case package (or a shared interfaces package) that consumes the
// validator. Use cases depend on that interface, not on this concrete struct,
// following the accept-interfaces-return-structs idiom and design.md §3.1.1.
type BookingValidator struct{}

// NewBookingValidator returns a stateless BookingValidator.
func NewBookingValidator() *BookingValidator { return &BookingValidator{} }

// ValidateBookingInput carries the proposed slot and all entities the use case
// has already resolved. No DB lookups happen inside Validate; it relies solely
// on the supplied BookingOverlapReader for the overlap step.
//
// Contract: Service and Professional MUST be non-nil — the helper requires
// Service for the step-4 slot-end computation and Professional for localized
// message generation. Passing a nil Professional is a programmer error.
type ValidateBookingInput struct {
	Service              *entity.Service
	Professional         *entity.Professional
	BusinessProfile      *entity.BusinessProfile
	ProfessionalSchedule *entity.Schedule               // for the slot's weekday
	Exception            *entity.BusinessHoursException // nil if none for the date
	NewStart             time.Time                      // already in business timezone semantics
	Bookings             BookingOverlapReader
}

// Validate runs the 5-step chain via the shared helper and returns the first
// *domain.SemanticError, or nil on success (REQ-BV-2, REQ-BV-4). It returns the
// helper result unchanged.
//
// It does NOT check service/professional active status — that responsibility
// stays in the use case (REQ-BV-4 failure modes). It assumes all entities in
// the input are already resolved and valid for the tenant.
func (v *BookingValidator) Validate(ctx context.Context, input ValidateBookingInput) *domain.SemanticError {
	return ValidateBookingTimeSlot(ctx, SlotInput{
		ProfessionalID:  input.Professional.ID,
		Service:         input.Service,
		Professional:    input.Professional,
		BusinessProfile: input.BusinessProfile,
		Schedule:        input.ProfessionalSchedule,
		Exception:       input.Exception,
		Start:           input.NewStart,
	}, BookingTimeValidatorDeps{Bookings: input.Bookings})
}

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// internalValidationMessage is the opaque, fail-closed message returned when the
// chain cannot reason about its inputs (a missing resolved entity or a malformed
// stored/extracted HH:MM value). It carries no paths, field dumps, or raw values.
const internalValidationMessage = "Error interno al validar el horario."

// internalValidationError wraps a non-business failure into the chain's internal
// SemanticError. The cause stays server-side for logging and never reaches the
// upstream LLM through Message.
func internalValidationError(cause error) *domain.SemanticError {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInternal,
		Message: internalValidationMessage,
		Cause:   cause,
	}
}

// BookingOverlapReader is the narrow read interface the validation chain needs.
// It is a subset of internal/domain/repository.BookingsRepo exposing only the
// overlap query. It is defined locally so the helper depends only on the method
// it actually calls, decoupling it from the full BookingsRepo and from any
// concrete SQL implementation (accept-interfaces-return-structs; R7 mitigation).
type BookingOverlapReader interface {
	FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error)
}

// SlotInput is the proposed slot expressed in business-local terms plus the
// resolved entities and identifiers the chain needs to produce localized
// messages. Every entity is assumed already resolved by the caller; the helper
// performs no lookups itself.
//
// Contract: Professional, BusinessProfile, and Service MUST be non-nil, and
// Service MUST have a positive duration. A violation is a programmer error and
// makes the chain fail closed with an internal error (it never panics, see
// ValidateBookingTimeSlot).
type SlotInput struct {
	ProfessionalID  string
	Service         *entity.Service
	Professional    *entity.Professional
	BusinessProfile *entity.BusinessProfile
	Schedule        *entity.Schedule
	Exception       *entity.BusinessHoursException // nil == no exception for the date
	Start           time.Time                      // parsed, in business *time.Location
}

// BookingTimeValidatorDeps groups the read-side dependencies for the chain.
type BookingTimeValidatorDeps struct {
	Bookings BookingOverlapReader
}

// ValidateBookingTimeSlot runs the 5-step booking-datetime validation chain in
// deterministic order (REQ-BTV-2):
//  1. past time check
//  2. business hours check (exception-aware, then JSON weekly schedule)
//  3. professional schedule check
//  4. slot-within-combined-hours check
//  5. overlap check via Bookings.FindOverlapping
//
// It returns the first *domain.SemanticError encountered, or nil on success,
// and short-circuits after the first failing step (REQ-BTV-3). It is a pure
// helper: performs no I/O and holds no state.
//
// The Service duration is a required input for step 4. Passing a nil Service or
// a Service with non-positive Duration is a programmer error (contract
// violation) that returns an internal *domain.SemanticError instead of
// panicking.
func ValidateBookingTimeSlot(ctx context.Context, slot SlotInput, deps BookingTimeValidatorDeps) *domain.SemanticError {
	// Defense in depth (zero trust): the steps below dereference these resolved
	// entities (BusinessProfile for the weekly schedule, Professional for the
	// localized messages). A nil one is a caller contract violation, so the
	// chain fails closed before comparing anything.
	if slot.BusinessProfile == nil || slot.Professional == nil {
		return internalValidationError(errors.New("slot.BusinessProfile y slot.Professional deben ser no nulos"))
	}

	// ─── Step 1 — Past time check ────────────────────────────────────────
	// "Now" is expressed in the business location so the comparison is
	// independent of the server's local zone (both operands are the same
	// instant; the zone only makes the intent explicit).
	now := time.Now().In(slot.Start.Location())
	if slot.Start.Before(now) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotInPast,
			Message: "No se puede reservar en el pasado.",
		}
	}

	// dayOfWeek keeps Go's time.Weekday encoding (0..6, Sunday=0): the one used
	// by schedules.day_of_week and by spanishDayNamesPlural.
	dayOfWeek := int(slot.Start.Weekday())
	// profileDayKey is the business_hours JSON encoding ("1".."7", Monday=1);
	// entity.ProfileDayKey is the single translation point between both.
	profileDayKey := entity.ProfileDayKey(slot.Start.Weekday())
	dateStr := slot.Start.Format("2006-01-02")

	// ─── Step 2 — Business hours (exception-aware, then weekly JSON) ─────
	var businessOpenHHMM, businessCloseHHMM string
	if slot.Exception != nil {
		if slot.Exception.IsClosedDay() {
			reason := ""
			if slot.Exception.Reason != nil {
				reason = *slot.Exception.Reason
			}
			return &domain.SemanticError{
				Code:    domain.ErrCodeBusinessClosed,
				Message: fmt.Sprintf("Negocio está cerrado el %s (%s).", dateStr, reason),
			}
		}
		if open, close, ok := slot.Exception.EffectiveHours(); ok {
			businessOpenHHMM = open
			businessCloseHHMM = close
		}
	}

	if businessOpenHHMM == "" || businessCloseHHMM == "" {
		open, close, ok := slot.BusinessProfile.GetOpenClose(profileDayKey)
		if !ok || open == "" || close == "" {
			return &domain.SemanticError{
				Code:    domain.ErrCodeBusinessClosed,
				Message: fmt.Sprintf("Negocio no abre los %s.", spanishDayNamesPlural[dayOfWeek]),
			}
		}
		businessOpenHHMM = open
		businessCloseHHMM = close
	}

	// ─── Step 3 — Professional schedule ──────────────────────────────────
	if slot.Schedule == nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeProfessionalNotWorking,
			Message: fmt.Sprintf("Profesional %s no trabaja los %s.", slot.Professional.Name, spanishDayNamesPlural[dayOfWeek]),
		}
	}
	proStartHHMM := slot.Schedule.StartTime
	proEndHHMM := slot.Schedule.EndTime

	// ─── Step 4 — Slot within combined business + professional hours ─────
	// A nil Service or a non-positive duration is a caller contract violation:
	// step 4 cannot compute the slot end, so it fails closed instead of panicking.
	if slot.Service == nil || slot.Service.Duration() <= 0 {
		return internalValidationError(errors.New("slot.Service debe ser no nulo y con duración positiva"))
	}
	slotStartHHMM := slot.Start.Format("15:04")
	slotEndHHMM := slot.Start.Add(slot.Service.Duration()).Format("15:04")

	// Every HH:MM bound is converted to minutes-since-midnight before being
	// compared. Comparing the raw strings only happens to work for well-formed
	// zero-padded values and would silently misjudge a malformed stored value;
	// any parse failure is a fail-closed internal error.
	businessOpenMin, err := hhmmToMinutes(businessOpenHHMM)
	if err != nil {
		return internalValidationError(err)
	}
	businessCloseMin, err := hhmmToMinutes(businessCloseHHMM)
	if err != nil {
		return internalValidationError(err)
	}
	proStartMin, err := hhmmToMinutes(proStartHHMM)
	if err != nil {
		return internalValidationError(err)
	}
	proEndMin, err := hhmmToMinutes(proEndHHMM)
	if err != nil {
		return internalValidationError(err)
	}
	slotStartMin, err := hhmmToMinutes(slotStartHHMM)
	if err != nil {
		return internalValidationError(err)
	}
	slotEndMin, err := hhmmToMinutes(slotEndHHMM)
	if err != nil {
		return internalValidationError(err)
	}

	effectiveCloseMin := businessCloseMin
	effectiveCloseHHMM := businessCloseHHMM
	if proEndMin < effectiveCloseMin {
		effectiveCloseMin = proEndMin
		effectiveCloseHHMM = proEndHHMM
	}

	// 4.1 — Slot ends after the effective close?
	if slotEndMin > effectiveCloseMin {
		remaining := effectiveCloseMin - slotStartMin
		if remaining < 0 {
			remaining = 0
		}
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Servicio dura %d minutos pero solo quedan %d antes del cierre a las %s.", slot.Service.DurationMinutes, remaining, effectiveCloseHHMM),
		}
	}

	// 4.2 — Slot starts before the business opening?
	if slotStartMin < businessOpenMin {
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Horario de atención comienza a las %s.", businessOpenHHMM),
		}
	}

	// 4.3 — Slot starts before the professional's start?
	if slotStartMin < proStartMin {
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Profesional %s empieza a las %s.", slot.Professional.Name, proStartHHMM),
		}
	}

	// ─── Step 5 — Overlap check ──────────────────────────────────────────
	startUTC := slot.Start.UTC()
	endUTC := slot.Start.Add(slot.Service.Duration()).UTC()
	overlapping, err := deps.Bookings.FindOverlapping(ctx, slot.ProfessionalID, startUTC, endUTC)
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "No se pudo verificar el turno: error al consultar reservas existentes.",
			Cause:   err,
		}
	}
	if len(overlapping) > 0 {
		existing := overlapping[0]
		return &domain.SemanticError{
			Code: domain.ErrCodeBookingOverlap,
			Message: fmt.Sprintf("Profesional %s ya tiene una reserva de %s a %s.",
				slot.Professional.Name,
				existing.StartDatetime.UTC().Format(time.RFC3339),
				existing.EndDatetime.UTC().Format(time.RFC3339)),
		}
	}

	return nil
}

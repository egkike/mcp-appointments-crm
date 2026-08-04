package service

import (
	"context"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

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
// Contract: Professional MUST be non-nil (used for the professional-schedule
// and overlap messages); Service MUST be non-nil with positive duration for
// step 4 (nil or non-positive panics, see ValidateBookingTimeSlot).
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
// violation) and panics — deliberately not a *domain.SemanticError.
func ValidateBookingTimeSlot(ctx context.Context, slot SlotInput, deps BookingTimeValidatorDeps) *domain.SemanticError {
	// ─── Step 1 — Past time check ────────────────────────────────────────
	if slot.Start.Before(time.Now()) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotInPast,
			Message: "No se puede reservar en el pasado.",
		}
	}

	dayOfWeek := int(slot.Start.Weekday())
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
		open, close, ok := slot.BusinessProfile.GetOpenClose(dayOfWeek)
		if !ok || open == "" || close == "" {
			return &domain.SemanticError{
				Code:    domain.ErrCodeBusinessClosed,
				Message: fmt.Sprintf("Negocio no abre los %s.", spanishDayNames[dayOfWeek]),
			}
		}
		businessOpenHHMM = open
		businessCloseHHMM = close
	}

	// ─── Step 3 — Professional schedule ──────────────────────────────────
	if slot.Schedule == nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeProfessionalNotWorking,
			Message: fmt.Sprintf("Profesional %s no trabaja los %s.", slot.Professional.Name, spanishDayNames[dayOfWeek]),
		}
	}
	proStartHHMM := slot.Schedule.StartTime
	proEndHHMM := slot.Schedule.EndTime

	// ─── Step 4 — Slot within combined business + professional hours ─────
	if slot.Service == nil || slot.Service.Duration() <= 0 {
		panic("ValidateBookingTimeSlot: slot.Service no puede ser nil y debe tener una duración positiva")
	}
	slotStartHHMM := slot.Start.Format("15:04")
	slotEndHHMM := slot.Start.Add(slot.Service.Duration()).Format("15:04")

	effectiveCloseHHMM := businessCloseHHMM
	if proEndHHMM < effectiveCloseHHMM {
		effectiveCloseHHMM = proEndHHMM
	}

	// 4.1 — Slot ends after the effective close?
	if slotEndHHMM > effectiveCloseHHMM {
		slotMin, err := hhmmToMinutes(slotStartHHMM)
		if err != nil {
			return &domain.SemanticError{Code: domain.ErrCodeInternal, Message: "Error interno al validar el horario.", Cause: err}
		}
		closeMin, err := hhmmToMinutes(effectiveCloseHHMM)
		if err != nil {
			return &domain.SemanticError{Code: domain.ErrCodeInternal, Message: "Error interno al validar el horario.", Cause: err}
		}
		remaining := closeMin - slotMin
		if remaining < 0 {
			remaining = 0
		}
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Servicio dura %d minutos pero solo quedan %d antes del cierre a las %s.", slot.Service.DurationMinutes, remaining, effectiveCloseHHMM),
		}
	}

	// 4.2 — Slot starts before the business opening?
	if slotStartHHMM < businessOpenHHMM {
		return &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Horario de atención comienza a las %s.", businessOpenHHMM),
		}
	}

	// 4.3 — Slot starts before the professional's start?
	if slotStartHHMM < proStartHHMM {
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

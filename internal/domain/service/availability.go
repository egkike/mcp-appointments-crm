package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// AvailabilityChecker is the contract the use-case layer depends on.
// *AvailabilityService satisfies it via Go's structural typing.
type AvailabilityChecker interface {
	CheckAvailability(ctx context.Context, params *CheckAvailabilityParams, deps AvailabilityDeps) (*CheckAvailabilityResult, error)
}

// AvailabilityService runs the 5-step CheckAvailability validation chain
// (business hours, professional schedule, slot within hours, overlap, past).
// It is stateless: all dependencies (repository interfaces) are passed per-call.
type AvailabilityService struct{}

// NewAvailabilityService returns a stateless AvailabilityService.
func NewAvailabilityService() *AvailabilityService { return &AvailabilityService{} }

// CheckAvailabilityParams is the input for the chain.
type CheckAvailabilityParams struct {
	ServiceID      string
	ProfessionalID string
	StartDatetime  string // RFC3339, parsed in business_profile.timezone
}

// CheckAvailabilityResult is the output.
type CheckAvailabilityResult struct {
	Available bool
}

// AvailabilityDeps groups the repository interfaces the chain needs.
// Passed as a method argument so the service stays stateless and testable.
type AvailabilityDeps struct {
	Services                repository.ServicesRepo
	Professionals           repository.ProfessionalsRepo
	BusinessProfile         repository.BusinessProfileRepo
	BusinessHoursExceptions repository.BusinessHoursExceptionRepo
	Schedules               repository.SchedulesRepo
	Bookings                repository.BookingsRepo
}

// spanishDayNames maps Go time.Weekday (0=Sunday) to Spanish names.
var spanishDayNames = [7]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

// CheckAvailability runs the 5-step validation chain. On success returns
// &CheckAvailabilityResult{Available: true}. On first failure returns a
// *domain.SemanticError with the appropriate ErrCode.
func (s *AvailabilityService) CheckAvailability(
	ctx context.Context,
	params *CheckAvailabilityParams,
	deps AvailabilityDeps,
) (*CheckAvailabilityResult, error) {
	// ─── Input Resolution ────────────────────────────────────────────────

	svc, err := deps.Services.FindByID(ctx, params.ServiceID)
	if err != nil {
		return nil, fmt.Errorf("check_availability: consultar servicio: %w", err)
	}
	if !svc.IsActive() {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeServiceNotActive,
			Message: fmt.Sprintf("Servicio %s no está activo.", svc.Name),
		}
	}

	pro, err := deps.Professionals.FindByID(ctx, params.ProfessionalID)
	if err != nil {
		return nil, fmt.Errorf("check_availability: consultar profesional: %w", err)
	}
	if !pro.IsActive() {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeProfessionalNotActive,
			Message: fmt.Sprintf("Profesional %s no está activo.", pro.Name),
		}
	}

	profile, err := deps.BusinessProfile.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("check_availability: consultar zona horaria: %w", err)
	}

	loc, err := ParseBusinessTimezone(profile.Timezone)
	if err != nil {
		return nil, fmt.Errorf("check_availability: %w", err)
	}

	startTime, err := ParseStartDatetime(params.StartDatetime, loc)
	if err != nil {
		return nil, fmt.Errorf("check_availability: %w", err)
	}

	endTime := startTime.Add(time.Duration(svc.DurationMinutes) * time.Minute)

	localStart := startTime.In(loc)
	localEnd := endTime.In(loc)
	dayOfWeek := int(localStart.Weekday())
	slotStartHHMM := localStart.Format("15:04")
	slotEndHHMM := localEnd.Format("15:04")
	dateStr := localStart.Format("2006-01-02")

	// ─── Step 3a — Business hours ────────────────────────────────────────

	exceptionDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	exception, err := deps.BusinessHoursExceptions.Get(ctx, exceptionDate)
	if err == nil {
		if exception.IsClosedDay() {
			reason := ""
			if exception.Reason != nil {
				reason = *exception.Reason
			}
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeBusinessClosed,
				Message: fmt.Sprintf("Negocio está cerrado el %s (%s).", dateStr, reason),
			}
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("check_availability: consultar excepción: %w", err)
	}

	var businessOpenHHMM, businessCloseHHMM string

	if err == nil && !exception.IsClosedDay() {
		if open, close, ok := exception.EffectiveHours(); ok {
			businessOpenHHMM = open
			businessCloseHHMM = close
		}
	}

	if businessOpenHHMM == "" || businessCloseHHMM == "" {
		open, close, ok := profile.GetOpenClose(dayOfWeek)
		if !ok || open == "" || close == "" {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeBusinessClosed,
				Message: fmt.Sprintf("Negocio no abre los %s.", spanishDayNames[dayOfWeek]),
			}
		}
		businessOpenHHMM = open
		businessCloseHHMM = close
	}

	// ─── Step 3b — Professional schedule ─────────────────────────────────

	schedule, err := deps.Schedules.FindByProfessionalAndDay(ctx, params.ProfessionalID, dayOfWeek)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeProfessionalNotWorking,
				Message: fmt.Sprintf("Profesional %s no trabaja los %s.", pro.Name, spanishDayNames[dayOfWeek]),
			}
		}
		return nil, fmt.Errorf("check_availability: consultar horario del profesional: %w", err)
	}
	proStartHHMM := schedule.StartTime
	proEndHHMM := schedule.EndTime

	// ─── Step 3c — Slot within hours ─────────────────────────────────────

	effectiveCloseHHMM := businessCloseHHMM
	if proEndHHMM < effectiveCloseHHMM {
		effectiveCloseHHMM = proEndHHMM
	}

	// 3c.1 — Slot ends after effective close?
	if slotEndHHMM > effectiveCloseHHMM {
		slotMin, err := hhmmToMinutes(slotStartHHMM)
		if err != nil {
			return nil, fmt.Errorf("check_availability: %w", err)
		}
		closeMin, err := hhmmToMinutes(effectiveCloseHHMM)
		if err != nil {
			return nil, fmt.Errorf("check_availability: %w", err)
		}
		remaining := closeMin - slotMin
		if remaining < 0 {
			remaining = 0
		}
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Servicio dura %d minutos pero solo quedan %d antes del cierre a las %s.", svc.DurationMinutes, remaining, effectiveCloseHHMM),
		}
	}

	// 3c.2 — Slot starts before business opening?
	if slotStartHHMM < businessOpenHHMM {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Horario de atención comienza a las %s.", businessOpenHHMM),
		}
	}

	// 3c.3 — Slot starts before professional's start?
	if slotStartHHMM < proStartHHMM {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeSlotOutOfHours,
			Message: fmt.Sprintf("Profesional %s empieza a las %s.", pro.Name, proStartHHMM),
		}
	}

	// ─── Step 3d — Overlap check ─────────────────────────────────────────

	startUTC := startTime.UTC()
	endUTC := endTime.UTC()

	overlapping, err := deps.Bookings.FindOverlapping(ctx, params.ProfessionalID, startUTC, endUTC)
	if err != nil {
		return nil, fmt.Errorf("check_availability: consultar overlap: %w", err)
	}
	if len(overlapping) > 0 {
		existing := overlapping[0]
		return nil, &domain.SemanticError{
			Code: domain.ErrCodeBookingOverlap,
			Message: fmt.Sprintf("Profesional %s ya tiene una reserva de %s a %s.",
				pro.Name,
				existing.StartDatetime.UTC().Format(time.RFC3339),
				existing.EndDatetime.UTC().Format(time.RFC3339)),
		}
	}

	// ─── Step 3e — Past check ────────────────────────────────────────────

	if startTime.Before(time.Now()) {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeSlotInPast,
			Message: "No se puede reservar en el pasado.",
		}
	}

	return &CheckAvailabilityResult{Available: true}, nil
}

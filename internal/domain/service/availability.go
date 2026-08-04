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

	localStart := startTime.In(loc)
	dayOfWeek := int(localStart.Weekday())

	exceptionDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	exception, err := deps.BusinessHoursExceptions.Get(ctx, exceptionDate)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("check_availability: consultar excepción: %w", err)
	}

	schedule, err := deps.Schedules.FindByProfessionalAndDay(ctx, params.ProfessionalID, dayOfWeek)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("check_availability: consultar horario del profesional: %w", err)
	}

	// ─── 5-step chain — delegated to the shared helper (REQ-AV-2) ─────────
	if semErr := ValidateBookingTimeSlot(ctx, SlotInput{
		ProfessionalID:  params.ProfessionalID,
		Service:         svc,
		Professional:    pro,
		BusinessProfile: profile,
		Schedule:        schedule,
		Exception:       exception,
		Start:           startTime,
	}, BookingTimeValidatorDeps{Bookings: deps.Bookings}); semErr != nil {
		return nil, semErr
	}

	return &CheckAvailabilityResult{Available: true}, nil
}

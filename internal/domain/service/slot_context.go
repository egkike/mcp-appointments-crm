package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepository "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// SlotContext bundles the entities a booking-slot validation needs:
// the professional, the business profile (timezone), the per-date exception
// and the professional's weekly schedule for the slot's weekday.
type SlotContext struct {
	// Professional is the staff member whose slot is validated.
	Professional *entity.Professional
	// Profile is the singleton business profile (timezone source).
	Profile *entity.BusinessProfile
	// Exception is the per-date exception for the slot, nil when the date
	// has none.
	Exception *entity.BusinessHoursException
	// Schedule is the professional's weekly schedule for the slot's
	// weekday, nil when none exists.
	Schedule *entity.Schedule
	// LocalStart is the requested start converted to the business timezone
	// (the value validators must receive, REQ-BV-4).
	LocalStart time.Time
}

// ResolveSlotContext resolves the entities a booking-slot validation needs in
// the exact order required by REQ-BV-4, shared by CreateBookingUseCase and
// RescheduleBookingUseCase (GGA: rule of three — AvailabilityService keeps
// its own variant because it additionally validates the service, resolves the
// start from an RFC3339 string and reports professional-not-found as an
// infrastructure error instead of a semantic one; converge it here when those
// differences disappear):
//
//  1. professional lookup — ErrNotFound → SemanticError ErrCodeNotFound
//     (fail-fast instead of a lookup that misreports the cause);
//  2. active-status check BEFORE the validator (availability.go:78-83), so
//     create/reschedule cannot accept a slot CheckAvailability rejects;
//  3. business profile (timezone);
//  4. business-timezone conversion and per-date exception lookup
//     (ErrNotFound tolerated: no exception that day);
//  5. weekly schedule for the slot's weekday (ErrNotFound tolerated: no
//     schedule that day).
//
// op is the caller's operation identity, used as the error-prefix context
// ("crear reserva:" vs "reprogramar reserva:") so each caller keeps its own
// error identity.
func ResolveSlotContext(
	ctx context.Context,
	op string,
	pros domainrepository.ProfessionalsRepo,
	profileRepo domainrepository.BusinessProfileRepo,
	exceptions domainrepository.BusinessHoursExceptionRepo,
	schedules domainrepository.SchedulesRepo,
	professionalID string,
	start time.Time,
) (*SlotContext, error) {
	pro, err := pros.FindByID(ctx, professionalID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: fmt.Sprintf("profesional %s no encontrado", professionalID), Cause: err}
		}
		return nil, fmt.Errorf("%s: consultar profesional: %w", op, err)
	}
	if !pro.IsActive() {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeProfessionalNotActive,
			Message: fmt.Sprintf("Profesional %s no está activo", pro.Name),
		}
	}

	profile, err := profileRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: consultar perfil de negocio: %w", op, err)
	}

	loc, err := ParseBusinessTimezone(profile.Timezone)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	localStart := start.In(loc)
	dayOfWeek := int(localStart.Weekday())
	exceptionDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	exception, err := exceptions.Get(ctx, exceptionDate)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%s: consultar excepción: %w", op, err)
	}

	schedule, err := schedules.FindByProfessionalAndDay(ctx, professionalID, dayOfWeek)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%s: consultar horario del profesional: %w", op, err)
	}

	return &SlotContext{
		Professional: pro,
		Profile:      profile,
		Exception:    exception,
		Schedule:     schedule,
		LocalStart:   localStart,
	}, nil
}

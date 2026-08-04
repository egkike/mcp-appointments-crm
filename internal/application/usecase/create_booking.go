// Package usecase implements application use cases (design D6).
// Each file contains one exported struct, one constructor (New<TypeName>),
// and one Execute method. Shared auth helpers live in authz.go.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
	"github.com/egkike/mcp-appointments-crm/internal/idgen"
)

// bookingValidator is the narrow contract the use case needs for datetime
// validation. The concrete *service.BookingValidator satisfies it
// structurally; tests inject a function-table mock (mockBookingValidator).
// The consumer-facing interface (domain.BookingValidator) is deferred to a
// later cleanup, so this local interface keeps the dependency mockable while
// following accept-interfaces-return-structs.
type bookingValidator interface {
	Validate(ctx context.Context, input service.ValidateBookingInput) *domain.SemanticError
}

// CreateBookingUseCase creates a new booking after authorization.
type CreateBookingUseCase struct {
	bookings  repository.BookingsRepo
	services  repository.ServicesRepo
	pros      repository.ProfessionalsRepo
	bizProf   repository.BusinessProfileRepo
	bizEx     repository.BusinessHoursExceptionRepo
	schedules repository.SchedulesRepo
	validator bookingValidator
}

// NewCreateBookingUseCase constructs a CreateBookingUseCase with the given dependencies.
//
// The four extra repos (professionals, business profile, business-hours
// exception, schedules) are required for datetime entity resolution BEFORE the
// validator call (design.md §3.4). The validator is accepted as the narrow
// bookingValidator interface so tests can inject a mock; production wiring
// (P4) passes the concrete *service.BookingValidator.
func NewCreateBookingUseCase(
	bookings repository.BookingsRepo,
	services repository.ServicesRepo,
	pros repository.ProfessionalsRepo,
	bizProf repository.BusinessProfileRepo,
	bizEx repository.BusinessHoursExceptionRepo,
	schedules repository.SchedulesRepo,
	validator bookingValidator,
) *CreateBookingUseCase {
	return &CreateBookingUseCase{
		bookings:  bookings,
		services:  services,
		pros:      pros,
		bizProf:   bizProf,
		bizEx:     bizEx,
		schedules: schedules,
		validator: validator,
	}
}

// Execute creates a booking. Caller must be authenticated; clients book for
// themselves, staff for their professional, admin/owner for anyone.
// Returns the new booking ID or a *domain.SemanticError.
func (uc *CreateBookingUseCase) Execute(ctx context.Context, input dto.CreateBookingInput) (*dto.CreateBookingResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	switch input.Caller.Role {
	case auth.RoleClient:
		if input.Caller.ClientID == nil || *input.Caller.ClientID != input.ClientID {
			return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "Cliente solo puede crear reservas para sí mismo", Cause: domain.ErrForbidden}
		}
	case auth.RoleStaff:
		if input.Caller.ProfessionalID == nil || *input.Caller.ProfessionalID != input.ProfessionalID {
			return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "Personal solo puede crear reservas para su profesional asignado", Cause: domain.ErrForbidden}
		}
	case auth.RoleAdmin, auth.RoleOwner:
	default:
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: fmt.Sprintf("Rol %q no puede crear reservas", input.Caller.Role), Cause: domain.ErrForbidden}
	}

	// ─── Input validation ──────────────────────────────────────────────
	if input.ClientID == "" {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "Cliente es requerido"}
	}
	if input.ServiceID == "" {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "Servicio es requerido"}
	}
	if input.StartTime.IsZero() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "La fecha y hora de inicio es requerida"}
	}

	svc, err := uc.services.FindByID(ctx, input.ServiceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: fmt.Sprintf("servicio %s no encontrado", input.ServiceID), Cause: err}
		}
		return nil, fmt.Errorf("crear reserva: consultar servicio: %w", err)
	}
	if !svc.IsActive() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeServiceNotActive, Message: fmt.Sprintf("Servicio %s no está activo", svc.Name)}
	}

	// ─── Resolve datetime-validation entities BEFORE the validator ────────
	// Reuses the same resolution pattern as AvailabilityService: professional,
	// business profile (timezone), per-date exception, and the professional's
	// weekly schedule for the slot's weekday. The active-status check above
	// stays in the use case; the validator does NOT own it (REQ-BV-4 failure
	// modes).
	pro, err := uc.pros.FindByID(ctx, input.ProfessionalID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: fmt.Sprintf("profesional %s no encontrado", input.ProfessionalID), Cause: err}
		}
		return nil, fmt.Errorf("crear reserva: consultar profesional: %w", err)
	}
	// Active-status check BEFORE the validator (REQ-BV-4 failure modes). The
	// validator does NOT own this check, mirroring AvailabilityService at
	// availability.go:78-83. Without this guard, a booking could be created
	// for an inactive professional while CheckAvailability correctly rejects
	// the same slot — a semantic inconsistency across use cases.
	if !pro.IsActive() {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeProfessionalNotActive,
			Message: fmt.Sprintf("Profesional %s no está activo", pro.Name),
		}
	}

	profile, err := uc.bizProf.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("crear reserva: consultar perfil de negocio: %w", err)
	}

	loc, err := service.ParseBusinessTimezone(profile.Timezone)
	if err != nil {
		return nil, fmt.Errorf("crear reserva: %w", err)
	}

	localStart := input.StartTime.In(loc)
	dayOfWeek := int(localStart.Weekday())
	exceptionDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	exception, err := uc.bizEx.Get(ctx, exceptionDate)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("crear reserva: consultar excepción: %w", err)
	}

	schedule, err := uc.schedules.FindByProfessionalAndDay(ctx, input.ProfessionalID, dayOfWeek)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("crear reserva: consultar horario del profesional: %w", err)
	}

	// ─── Validate BEFORE repo dispatch (REQ-BK-9) ─────────────────────────
	// On validator error, return it unchanged (REQ-BK-10, REQ-BK-11): the use
	// case MUST NOT rewrap a *domain.SemanticError as domain.ErrConflict.
	if semErr := uc.validator.Validate(ctx, service.ValidateBookingInput{
		Service:              svc,
		Professional:         pro,
		BusinessProfile:      profile,
		ProfessionalSchedule: schedule,
		Exception:            exception,
		NewStart:             localStart,
		Bookings:             uc.bookings,
	}); semErr != nil {
		return nil, semErr
	}

	bookingID, err := idgen.New()
	if err != nil {
		return nil, fmt.Errorf("crear reserva: generar ID: %w", err)
	}
	booking := &entity.Booking{
		ID:             bookingID,
		ClientID:       input.ClientID,
		ProfessionalID: input.ProfessionalID,
		ServiceID:      input.ServiceID,
		StartDatetime:  input.StartTime,
		EndDatetime:    input.StartTime.Add(svc.Duration()),
		Status:         entity.BookingStatusPending,
		Notes:          input.Notes,
		PaymentMethod:  input.PaymentMethod,
	}
	if err := uc.bookings.Create(ctx, booking); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeBookingOverlap, Message: fmt.Sprintf("Profesional %s ya tiene una reserva en ese horario", input.ProfessionalID), Cause: err}
		}
		return nil, fmt.Errorf("crear reserva: %w", err)
	}
	return &dto.CreateBookingResult{BookingID: booking.ID}, nil
}

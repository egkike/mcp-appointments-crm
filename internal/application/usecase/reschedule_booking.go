package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// bookingValidator is declared in create_booking.go and shared by both use
// cases (same package).

// RescheduleBookingUseCase reschedules a booking to a new start time.
type RescheduleBookingUseCase struct {
	bookings  repository.BookingsRepo
	services  repository.ServicesRepo
	pros      repository.ProfessionalsRepo
	bizProf   repository.BusinessProfileRepo
	bizEx     repository.BusinessHoursExceptionRepo
	schedules repository.SchedulesRepo
	validator bookingValidator
}

// NewRescheduleBookingUseCase constructs a RescheduleBookingUseCase with the given dependencies.
//
// The four extra repos (professionals, business profile, business-hours
// exception, schedules) are required for datetime entity resolution BEFORE the
// validator call (design.md §3.4). The validator is accepted as the narrow
// bookingValidator interface so tests can inject a mock; production wiring
// (P4) passes the concrete *service.BookingValidator.
func NewRescheduleBookingUseCase(
	bookings repository.BookingsRepo,
	services repository.ServicesRepo,
	pros repository.ProfessionalsRepo,
	bizProf repository.BusinessProfileRepo,
	bizEx repository.BusinessHoursExceptionRepo,
	schedules repository.SchedulesRepo,
	validator bookingValidator,
) *RescheduleBookingUseCase {
	return &RescheduleBookingUseCase{
		bookings:  bookings,
		services:  services,
		pros:      pros,
		bizProf:   bizProf,
		bizEx:     bizEx,
		schedules: schedules,
		validator: validator,
	}
}

// Execute reschedules the identified booking. End time is recomputed from
// service duration. Auth: same cross-tenant rules as cancel.
func (uc *RescheduleBookingUseCase) Execute(ctx context.Context, input dto.RescheduleBookingInput) (*dto.RescheduleBookingResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	// Empty BookingID fails fast with a semantic error instead of a lookup
	// that would misreport the cause as "reserva no encontrada" (GGA S-4).
	if input.BookingID == "" {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "Identificador de reserva requerido"}
	}
	booking, err := uc.bookings.FindByID(ctx, input.BookingID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada", Cause: err}
		}
		return nil, fmt.Errorf("reprogramar reserva: consultar: %w", err)
	}
	if err := auth.AuthorizeBookingAccess(input.Caller, booking); err != nil {
		return nil, err
	}
	if !booking.CanReschedule() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: fmt.Sprintf("La reserva en estado %q no puede ser reprogramada", booking.Status)}
	}
	if input.NewStartTime.IsZero() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "La nueva fecha y hora de inicio es requerida"}
	}

	svc, err := uc.services.FindByID(ctx, booking.ServiceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: fmt.Sprintf("servicio %s no encontrado", booking.ServiceID), Cause: err}
		}
		return nil, fmt.Errorf("reprogramar reserva: consultar servicio: %w", err)
	}
	if !svc.IsActive() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeServiceNotActive, Message: fmt.Sprintf("Servicio %s no está activo", svc.Name)}
	}

	// ─── Resolve datetime-validation entities BEFORE the validator ────────
	// Delegated to service.ResolveSlotContext (slot_context.go): professional
	// lookup + active check, business profile (timezone), per-date exception
	// and weekly schedule, in the REQ-BV-4 order. Shared with
	// CreateBookingUseCase so the sequence lives in one place.
	slot, err := service.ResolveSlotContext(ctx, "reprogramar reserva", uc.pros, uc.bizProf, uc.bizEx, uc.schedules, booking.ProfessionalID, input.NewStartTime)
	if err != nil {
		return nil, err
	}

	// ─── Validate BEFORE repo dispatch (REQ-BK-9) ─────────────────────────
	// On validator error, return it unchanged (REQ-BK-10, REQ-BK-11): the use
	// case MUST NOT rewrap a *domain.SemanticError as domain.ErrConflict.
	if semErr := uc.validator.Validate(ctx, service.ValidateBookingInput{
		Service:              svc,
		Professional:         slot.Professional,
		BusinessProfile:      slot.Profile,
		ProfessionalSchedule: slot.Schedule,
		Exception:            slot.Exception,
		NewStart:             slot.LocalStart,
		Bookings:             uc.bookings,
	}); semErr != nil {
		return nil, semErr
	}

	newEnd := input.NewStartTime.Add(svc.Duration())
	if err := uc.bookings.Reschedule(ctx, input.BookingID, input.NewStartTime, newEnd); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeBookingOverlap, Message: fmt.Sprintf("Profesional %s ya tiene una reserva en el nuevo horario", booking.ProfessionalID), Cause: err}
		}
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada", Cause: err}
		}
		return nil, fmt.Errorf("reprogramar reserva: %w", err)
	}
	// Status preserved: RescheduleBookingUseCase.Execute only changes the
	// start_datetime of an existing booking; it does NOT change the
	// booking's status (pending/confirmed/cancelled stays as-is). The
	// returned Status field therefore reflects the post-reschedule state,
	// which equals the pre-reschedule state by design.
	return &dto.RescheduleBookingResult{
		BookingID:     input.BookingID,
		Status:        string(booking.Status),
		StartDatetime: input.NewStartTime,
		EndDatetime:   newEnd,
	}, nil
}

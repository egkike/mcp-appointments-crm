// Package usecase implements application use cases (design D6).
// Each file contains one exported struct, one constructor (New<TypeName>),
// and one Execute method. Shared auth helpers live in authz.go.
package usecase

import (
	"context"
	"errors"
	"fmt"

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
// The interface is declared in validator.go (shared by both use cases) per
// the accept-interfaces-return-structs idiom; the consumer-facing interface
// (domain.BookingValidator) is deferred to a later cleanup.

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
	if input.ProfessionalID == "" {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: "Profesional es requerido"}
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
	// Delegated to service.ResolveSlotContext (slot_context.go): professional
	// lookup + active check, business profile (timezone), per-date exception
	// and weekly schedule, in the REQ-BV-4 order. Shared with
	// RescheduleBookingUseCase so the sequence lives in one place.
	slot, err := service.ResolveSlotContext(ctx, "crear reserva", uc.pros, uc.bizProf, uc.bizEx, uc.schedules, input.ProfessionalID, input.StartTime)
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
	return &dto.CreateBookingResult{
		BookingID:     booking.ID,
		StartDatetime: booking.StartDatetime,
		EndDatetime:   booking.EndDatetime,
	}, nil
}

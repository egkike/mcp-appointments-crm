package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// RescheduleBookingUseCase reschedules a booking to a new start time.
type RescheduleBookingUseCase struct {
	bookings repository.BookingsRepo
	services repository.ServicesRepo
}

// NewRescheduleBookingUseCase constructs a RescheduleBookingUseCase with the given dependencies.
func NewRescheduleBookingUseCase(bookings repository.BookingsRepo, services repository.ServicesRepo) *RescheduleBookingUseCase {
	return &RescheduleBookingUseCase{bookings: bookings, services: services}
}

// Execute reschedules the identified booking. End time is recomputed from
// service duration. Auth: same cross-tenant rules as cancel.
func (uc *RescheduleBookingUseCase) Execute(ctx context.Context, input dto.RescheduleBookingInput) (*dto.RescheduleBookingResult, error) {
	if err := auth.RequireAuthenticated(input.Caller); err != nil {
		return nil, err
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

	svc, err := uc.services.FindByID(ctx, booking.ServiceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: fmt.Sprintf("servicio %s no encontrado", booking.ServiceID), Cause: err}
		}
		return nil, fmt.Errorf("reprogramar reserva: consultar servicio: %w", err)
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
	return &dto.RescheduleBookingResult{BookingID: input.BookingID, Status: string(booking.Status)}, nil
}

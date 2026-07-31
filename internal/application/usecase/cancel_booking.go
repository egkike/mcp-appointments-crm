package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// CancelBookingUseCase cancels an existing booking after authorization.
type CancelBookingUseCase struct {
	bookings repository.BookingsRepo
}

// NewCancelBookingUseCase constructs a CancelBookingUseCase with the given dependencies.
func NewCancelBookingUseCase(bookings repository.BookingsRepo) *CancelBookingUseCase {
	return &CancelBookingUseCase{bookings: bookings}
}

// Execute cancels the identified booking. Caller must be authenticated and
// authorized (client owns, staff linked professional, admin/owner any).
func (uc *CancelBookingUseCase) Execute(ctx context.Context, input dto.CancelBookingInput) (*dto.CancelBookingResult, error) {
	if err := requireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	booking, err := uc.bookings.FindByID(ctx, input.BookingID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada", Cause: err}
		}
		return nil, fmt.Errorf("cancelar reserva: consultar: %w", err)
	}
	if err := authorizeBookingAccess(input.Caller, booking); err != nil {
		return nil, err
	}
	if !booking.CanCancel() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: fmt.Sprintf("la reserva en estado %q no puede ser cancelada", booking.Status)}
	}
	if err := uc.bookings.Cancel(ctx, input.BookingID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada", Cause: err}
		}
		return nil, fmt.Errorf("cancelar reserva: %w", err)
	}
	return &dto.CancelBookingResult{BookingID: input.BookingID, Status: string(entity.BookingStatusCancelled)}, nil
}

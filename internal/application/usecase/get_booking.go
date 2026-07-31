package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// GetBookingUseCase retrieves a single booking after authorization.
type GetBookingUseCase struct {
	bookings repository.BookingsRepo
}

// NewGetBookingUseCase constructs a GetBookingUseCase with the given dependencies.
func NewGetBookingUseCase(bookings repository.BookingsRepo) *GetBookingUseCase {
	return &GetBookingUseCase{bookings: bookings}
}

// Execute retrieves the identified booking. Auth: cross-tenant isolation
// (client owns, staff linked professional, admin/owner any).
func (uc *GetBookingUseCase) Execute(ctx context.Context, input dto.GetBookingInput) (*dto.GetBookingResult, error) {
	if err := requireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	booking, err := uc.bookings.FindByID(ctx, input.BookingID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada", Cause: err}
		}
		return nil, fmt.Errorf("obtener reserva: %w", err)
	}
	if err := authorizeBookingAccess(input.Caller, booking); err != nil {
		return nil, err
	}
	view := dto.BookingView{
		ID:             booking.ID,
		ClientID:       booking.ClientID,
		ProfessionalID: booking.ProfessionalID,
		ServiceID:      booking.ServiceID,
		StartDatetime:  booking.StartDatetime,
		EndDatetime:    booking.EndDatetime,
		Status:         string(booking.Status),
		Notes:          booking.Notes,
		PaymentMethod:  booking.PaymentMethod,
		CreatedAt:      booking.CreatedAt,
		UpdatedAt:      booking.UpdatedAt,
	}
	return &dto.GetBookingResult{Booking: view}, nil
}

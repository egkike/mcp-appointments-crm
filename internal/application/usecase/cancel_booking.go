package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// CancelBookingUseCase cancels an existing booking after authorization.
type CancelBookingUseCase struct {
	bookings repository.BookingsRepo
	alerts   AlertLifecycleStore
	logger   *slog.Logger
}

// NewCancelBookingUseCase constructs a CancelBookingUseCase with the given dependencies.
// The alerts port cancels any pending confirmation alert after the booking is
// cancelled. Pass nil to keep alert lifecycle disabled (skeleton behavior).
func NewCancelBookingUseCase(bookings repository.BookingsRepo, alerts AlertLifecycleStore, logger *slog.Logger) *CancelBookingUseCase {
	return &CancelBookingUseCase{bookings: bookings, alerts: alerts, logger: logger}
}

// Execute cancels the identified booking. Caller must be authenticated and
// authorized (client owns, staff linked professional, admin/owner any).
func (uc *CancelBookingUseCase) Execute(ctx context.Context, input dto.CancelBookingInput) (*dto.CancelBookingResult, error) {
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
		return nil, fmt.Errorf("cancelar reserva: consultar: %w", err)
	}
	if err := auth.AuthorizeBookingAccess(input.Caller, booking); err != nil {
		return nil, err
	}
	if !booking.CanCancel() {
		return nil, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: fmt.Sprintf("La reserva en estado %q no puede ser cancelada", booking.Status)}
	}
	if err := uc.bookings.Cancel(ctx, input.BookingID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada", Cause: err}
		}
		return nil, fmt.Errorf("cancelar reserva: %w", err)
	}
	uc.cancelAlert(ctx, input.BookingID)
	return &dto.CancelBookingResult{BookingID: input.BookingID, Status: string(entity.BookingStatusCancelled)}, nil
}

// cancelAlert cancels any pending confirmation alert linked to the booking.
// Failures are logged and intentionally do not affect the booking result.
func (uc *CancelBookingUseCase) cancelAlert(ctx context.Context, bookingID string) {
	if uc.alerts == nil {
		return
	}
	if err := uc.alerts.CancelByBookingID(ctx, bookingID); err != nil {
		LogAlertFailure(uc.logger, "cancel_alert", bookingID, err)
	}
}

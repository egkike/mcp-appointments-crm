// Package usecase implements application use cases (design D6).
// Each file contains one exported struct, one constructor (New<TypeName>),
// and one Execute method. Shared auth helpers live in authz.go.
package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// CreateBookingUseCase creates a new booking after authorization.
type CreateBookingUseCase struct {
	bookings repository.BookingsRepo
	services repository.ServicesRepo
}

// NewCreateBookingUseCase constructs a CreateBookingUseCase with the given dependencies.
func NewCreateBookingUseCase(bookings repository.BookingsRepo, services repository.ServicesRepo) *CreateBookingUseCase {
	return &CreateBookingUseCase{bookings: bookings, services: services}
}

// Execute creates a booking. Caller must be authenticated; clients book for
// themselves, staff for their professional, admin/owner for anyone.
// Returns the new booking ID or a *domain.SemanticError.
func (uc *CreateBookingUseCase) Execute(ctx context.Context, input dto.CreateBookingInput) (*dto.CreateBookingResult, error) {
	if err := requireAuthenticated(input.Caller); err != nil {
		return nil, err
	}
	switch input.Caller.Role {
	case auth.RoleClient:
		if input.Caller.ClientID == nil || *input.Caller.ClientID != input.ClientID {
			return nil, &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: "Cliente solo puede crear reservas para sí mismo", Cause: domain.ErrUnauthenticated}
		}
	case auth.RoleStaff:
		if input.Caller.ProfessionalID == nil || *input.Caller.ProfessionalID != input.ProfessionalID {
			return nil, &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: "Personal solo puede crear reservas para su profesional asignado", Cause: domain.ErrUnauthenticated}
		}
	case auth.RoleAdmin, auth.RoleOwner:
	default:
		return nil, &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: fmt.Sprintf("Rol %q no puede crear reservas", input.Caller.Role), Cause: domain.ErrUnauthenticated}
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

	booking := &entity.Booking{
		ID:             generateID(),
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

// generateID produces a UUID v4 string using crypto/rand.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

package usecase

import (
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// requireAuthenticated verifies the caller has a non-empty ID.
func requireAuthenticated(caller auth.Caller) error {
	if caller.ID == "" {
		return &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: "Usuario no autenticado", Cause: domain.ErrUnauthenticated}
	}
	return nil
}

// authorizeBookingAccess checks cross-tenant isolation: clients see only their
// bookings, staff see only their professional's bookings, admin/owner see all.
func authorizeBookingAccess(caller auth.Caller, booking *entity.Booking) error {
	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil || *caller.ClientID != booking.ClientID {
			return &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: "Cliente solo puede acceder a sus propias reservas", Cause: domain.ErrUnauthenticated}
		}
	case auth.RoleStaff:
		if caller.ProfessionalID == nil || *caller.ProfessionalID != booking.ProfessionalID {
			return &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: "Personal solo puede acceder a las reservas de su profesional asignado", Cause: domain.ErrUnauthenticated}
		}
	case auth.RoleAdmin, auth.RoleOwner:
	default:
		return &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: fmt.Sprintf("Rol %q no puede acceder a las reservas", caller.Role), Cause: domain.ErrUnauthenticated}
	}
	return nil
}

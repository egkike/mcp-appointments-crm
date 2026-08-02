package auth

import (
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// RequireAuthenticated verifies the caller has a non-empty ID. Use this for
// the "I need a logged-in user" precondition; the error code is
// ErrCodeUnauthenticated (truly missing auth, not just forbidden scope).
func RequireAuthenticated(caller Caller) error {
	if caller.ID == "" {
		return &domain.SemanticError{Code: domain.ErrCodeUnauthenticated, Message: "Usuario no autenticado", Cause: domain.ErrUnauthenticated}
	}
	return nil
}

// AuthorizeBookingAccess checks cross-tenant isolation for booking operations:
// clients see only their bookings, staff see only their professional's
// bookings, admin/owner see all. Returns ErrCodeForbidden for any cross-tenant
// denial (caller IS authenticated but lacks the scope to access this booking).
func AuthorizeBookingAccess(caller Caller, booking *entity.Booking) error {
	switch caller.Role {
	case RoleClient:
		if caller.ClientID == nil || *caller.ClientID != booking.ClientID {
			return &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "Cliente solo puede acceder a sus propias reservas", Cause: domain.ErrForbidden}
		}
	case RoleStaff:
		if caller.ProfessionalID == nil || *caller.ProfessionalID != booking.ProfessionalID {
			return &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "Personal solo puede acceder a las reservas de su profesional asignado", Cause: domain.ErrForbidden}
		}
	case RoleAdmin, RoleOwner:
	default:
		return &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: fmt.Sprintf("Rol %q no puede acceder a las reservas", caller.Role), Cause: domain.ErrForbidden}
	}
	return nil
}

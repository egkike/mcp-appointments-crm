package auth

import (
	"context"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// RequireCaller extracts the Caller from ctx.
// Returns *domain.SemanticError{Code: ErrCodeUnauthenticated} if no caller is present.
//
// NOTE: named RequireCaller (not actorFromContext) to avoid collision with the
// existing actorFromContext in accounts.go which returns a plain string for
// audit logging.
func RequireCaller(ctx context.Context) (*Caller, error) {
	caller, ok := FromContext(ctx)
	if !ok {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeUnauthenticated,
			Message: "se requiere autenticación",
			Cause:   domain.ErrUnauthenticated,
		}
	}
	return &caller, nil
}

// RequireRole checks that the caller's role is in the allowed set.
// Returns *domain.SemanticError{Code: ErrCodeUnauthenticated} if no caller is present
// or the caller's role is not authorized.
func RequireRole(ctx context.Context, roles ...string) (*Caller, error) {
	caller, err := RequireCaller(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if caller.Role == r {
			return caller, nil
		}
	}
	return nil, &domain.SemanticError{
		Code:    domain.ErrCodeUnauthenticated,
		Message: fmt.Sprintf("Rol %q no tiene permiso para esta operación", caller.Role),
		Cause:   domain.ErrUnauthenticated,
	}
}

// RequireClientMatch asserts that the caller is authorized to act on behalf of
// the given clientID for the given professionalID. Admin and owner roles bypass
// the check. Staff roles must have caller.ProfessionalID == inputProfessionalID
// (they can only create bookings on their own calendar). Client roles must have
// caller.ClientID == inputClientID.
func RequireClientMatch(ctx context.Context, inputClientID, inputProfessionalID string) error {
	caller, err := RequireCaller(ctx)
	if err != nil {
		return err
	}
	// Admin/owner bypass — full access
	if caller.Role == RoleAdmin || caller.Role == RoleOwner {
		return nil
	}
	// Staff must match their own professional (prevents calendar planting)
	if caller.Role == RoleStaff {
		if caller.ProfessionalID == nil || *caller.ProfessionalID != inputProfessionalID {
			return &domain.SemanticError{
				Code:    domain.ErrCodeUnauthenticated,
				Message: "Profesional no tiene permiso para operar en este calendario",
				Cause:   domain.ErrUnauthenticated,
			}
		}
		return nil
	}
	// Client must match their own ID
	if caller.Role == RoleClient {
		if caller.ClientID == nil || *caller.ClientID != inputClientID {
			return &domain.SemanticError{
				Code:    domain.ErrCodeUnauthenticated,
				Message: "no tiene permiso para operar en nombre de otro cliente",
				Cause:   domain.ErrUnauthenticated,
			}
		}
		return nil
	}
	// Unknown role — deny
	return &domain.SemanticError{
		Code:    domain.ErrCodeUnauthenticated,
		Message: fmt.Sprintf("Rol %q no tiene permiso para esta operación", caller.Role),
		Cause:   domain.ErrUnauthenticated,
	}
}

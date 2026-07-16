package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// ErrUnauthenticated is returned when a repository method requires an
// authenticated caller but none is present in the context, or the caller's
// role is not authorized for the operation.
var ErrUnauthenticated = errors.New("caller not authenticated")

// requireCaller extracts the auth.Caller from ctx.
// Returns *SemanticError{Code: ErrCodeUnauthenticated} if no caller is present.
//
// NOTE: named requireCaller (not actorFromContext) to avoid collision with the
// existing actorFromContext in accounts.go which returns a plain string for
// audit logging.
func requireCaller(ctx context.Context) (*auth.Caller, error) {
	caller, ok := auth.FromContext(ctx)
	if !ok {
		return nil, &SemanticError{
			Code:    ErrCodeUnauthenticated,
			Message: "se requiere autenticación",
			Cause:   ErrUnauthenticated,
		}
	}
	return &caller, nil
}

// requireRole checks that the caller's role is in the allowed set.
// Returns *SemanticError{Code: ErrCodeUnauthenticated} if no caller is present
// or the caller's role is not authorized.
func requireRole(ctx context.Context, roles ...string) (*auth.Caller, error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if caller.Role == r {
			return caller, nil
		}
	}
	return nil, &SemanticError{
		Code:    ErrCodeUnauthenticated,
		Message: fmt.Sprintf("el rol %q no tiene permiso para esta operación", caller.Role),
		Cause:   ErrUnauthenticated,
	}
}

// requireClientMatch asserts that the caller is authorized to act on behalf of
// the given clientID for the given professionalID. Admin and owner roles bypass
// the check. Staff roles must have caller.ProfessionalID == inputProfessionalID
// (they can only create bookings on their own calendar). Client roles must have
// caller.ClientID == inputClientID.
func requireClientMatch(ctx context.Context, inputClientID, inputProfessionalID string) error {
	caller, err := requireCaller(ctx)
	if err != nil {
		return err
	}
	// Admin/owner bypass — full access
	if caller.Role == auth.RoleAdmin || caller.Role == auth.RoleOwner {
		return nil
	}
	// Staff must match their own professional (prevents calendar planting)
	if caller.Role == auth.RoleStaff {
		if caller.ProfessionalID == nil || *caller.ProfessionalID != inputProfessionalID {
			return &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el profesional no tiene permiso para operar en este calendario",
				Cause:   ErrUnauthenticated,
			}
		}
		return nil
	}
	// Client must match their own ID
	if caller.Role == auth.RoleClient {
		if caller.ClientID == nil || *caller.ClientID != inputClientID {
			return &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "no tiene permiso para operar en nombre de otro cliente",
				Cause:   ErrUnauthenticated,
			}
		}
		return nil
	}
	// Unknown role — deny
	return &SemanticError{
		Code:    ErrCodeUnauthenticated,
		Message: fmt.Sprintf("el rol %q no tiene permiso para esta operación", caller.Role),
		Cause:   ErrUnauthenticated,
	}
}

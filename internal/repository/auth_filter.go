package repository

import (
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// applyAuthFilter modifies a SQL query and args based on the caller's role.
// It appends a role-specific WHERE clause (client_id or professional_id) for
// scoped roles, leaves the query unchanged for admin/owner, and returns a
// domain error for unknown roles or missing role-specific IDs.
//
// The original baseArgs slice is never mutated; a defensive copy is returned.
func applyAuthFilter(caller *auth.Caller, baseQuery string, baseArgs []any) (string, []any, error) {
	if caller == nil {
		return "", nil, &domain.SemanticError{
			Code:    domain.ErrCodeUnauthenticated,
			Message: "se requiere autenticación",
			Cause:   domain.ErrUnauthenticated,
		}
	}

	// Defensive copy — never mutate the caller's slice.
	args := make([]any, len(baseArgs))
	copy(args, baseArgs)
	query := baseQuery

	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil {
			return "", nil, &domain.SemanticError{
				Code:    domain.ErrCodeUnauthenticated,
				Message: "Cliente no tiene ID asignado",
				Cause:   domain.ErrUnauthenticated,
			}
		}
		query += " AND client_id = ?"
		args = append(args, *caller.ClientID)
	case auth.RoleStaff:
		if caller.ProfessionalID == nil {
			return "", nil, &domain.SemanticError{
				Code:    domain.ErrCodeUnauthenticated,
				Message: "Profesional no tiene ID asignado",
				Cause:   domain.ErrUnauthenticated,
			}
		}
		query += " AND professional_id = ?"
		args = append(args, *caller.ProfessionalID)
	case auth.RoleAdmin, auth.RoleOwner:
		// no extra filter
	default:
		return "", nil, &domain.SemanticError{
			Code:    domain.ErrCodeUnauthenticated,
			Message: fmt.Sprintf("Rol %q no tiene permiso para acceder a reservas", caller.Role),
			Cause:   domain.ErrUnauthenticated,
		}
	}

	return query, args, nil
}

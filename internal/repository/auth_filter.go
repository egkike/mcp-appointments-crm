package repository

import (
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// applyAuthFilter modifies a SQL query and args based on the caller's role.
// It inserts a role-specific WHERE clause (client_id or professional_id) for
// scoped roles, leaves the query unchanged for admin/owner, and returns a
// domain error for unknown roles or missing role-specific IDs.
//
// For queries that contain ORDER BY or LIMIT clauses, the auth clause is
// inserted BEFORE the clause (since WHERE cannot follow ORDER BY/LIMIT in SQL).
// For queries without ORDER BY/LIMIT, the auth clause is appended to the end.
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
	// Preallocate capacity for the potential new auth arg to avoid realloc.
	args := make([]any, len(baseArgs), len(baseArgs)+1)
	copy(args, baseArgs)
	query := baseQuery

	var filterClause string
	var filterArg any
	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil {
			return "", nil, &domain.SemanticError{
				Code:    domain.ErrCodeForbidden,
				Message: "Cliente no tiene ID asignado",
				Cause:   domain.ErrForbidden,
			}
		}
		filterClause = " AND client_id = ?"
		filterArg = *caller.ClientID
	case auth.RoleStaff:
		if caller.ProfessionalID == nil {
			return "", nil, &domain.SemanticError{
				Code:    domain.ErrCodeForbidden,
				Message: "Profesional no tiene ID asignado",
				Cause:   domain.ErrForbidden,
			}
		}
		filterClause = " AND professional_id = ?"
		filterArg = *caller.ProfessionalID
	case auth.RoleAdmin, auth.RoleOwner:
		// no extra filter
	default:
		return "", nil, &domain.SemanticError{
			Code:    domain.ErrCodeForbidden,
			Message: fmt.Sprintf("Rol %q no tiene permiso para acceder a reservas", caller.Role),
			Cause:   domain.ErrForbidden,
		}
	}

	// Admin/owner: no clause, no change.
	if filterClause == "" {
		return query, args, nil
	}

	// Find the position to insert the auth clause. WHERE cannot follow
	// ORDER BY or LIMIT in SQL, so the clause must be inserted BEFORE the
	// last occurrence of either keyword. Use case-insensitive LastIndex.
	// Use the uppercased query only for searching; insert positions are
	// then mapped back to the original (unmodified) query.
	upper := strings.ToUpper(query)
	insertPos := len(query)
	if idx := strings.LastIndex(upper, "ORDER BY"); idx >= 0 {
		insertPos = idx
	}
	if idx := strings.LastIndex(upper, "LIMIT"); idx >= 0 && idx < insertPos {
		insertPos = idx
	}

	suffix := query[insertPos:]
	if suffix != "" {
		// Inserting mid-query: separate the filter clause from the next
		// clause (ORDER BY, LIMIT, etc.) with a single space.
		query = query[:insertPos] + filterClause + " " + suffix
	} else {
		// Appending to end: no trailing space.
		query = query[:insertPos] + filterClause
	}
	args = append(args, filterArg)
	return query, args, nil
}

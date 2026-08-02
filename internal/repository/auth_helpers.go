package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// ErrUnauthenticated is returned when a repository method requires an
// authenticated caller but none is present in the context, or the caller's
// role is not authorized for the operation.
//
// NOTE: This is a package-level alias for domain.ErrUnauthenticated.
// It will be removed in P3.1c — consumers should use domain.ErrUnauthenticated.
var ErrUnauthenticated = domain.ErrUnauthenticated

// requireCaller extracts the auth.Caller from ctx.
// Delegates to auth.RequireCaller (P3.1b migration).
//
// NOTE: named requireCaller (not actorFromContext) to avoid collision with the
// existing actorFromContext in accounts.go which returns a plain string for
// audit logging.
func requireCaller(ctx context.Context) (*auth.Caller, error) {
	return auth.RequireCaller(ctx)
}

// requireRole checks that the caller's role is in the allowed set.
// Delegates to auth.RequireRole (P3.1b migration).
func requireRole(ctx context.Context, roles ...string) (*auth.Caller, error) {
	return auth.RequireRole(ctx, roles...)
}

// requireClientMatch asserts that the caller is authorized to act on behalf of
// the given clientID for the given professionalID.
// Delegates to auth.RequireClientMatch (P3.1b migration).
func requireClientMatch(ctx context.Context, inputClientID, inputProfessionalID string) error {
	return auth.RequireClientMatch(ctx, inputClientID, inputProfessionalID)
}

package auth

import "context"

// Canonical role names for the authorization model.
const (
	// RoleOwner is the role assigned to the single owner of the system.
	RoleOwner = "owner"
	// RoleAdmin is the role for administrative staff (subset of owner powers).
	RoleAdmin = "admin"
	// RoleStaff is the role for service professionals.
	RoleStaff = "staff"
	// RoleClient is the role for end customers, identified by presence in the clients table.
	RoleClient = "client"
)

// Caller represents an authenticated caller in the system.
type Caller struct {
	ID             string
	Role           string
	ProfessionalID *string
	ClientID       *string
}

// callerKey is the private context key for storing/retrieving Caller.
type callerKey struct{}

// WithCaller returns a new context carrying the given Caller.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, caller)
}

// FromContext extracts the Caller from ctx. Returns (Caller{}, false) if absent.
func FromContext(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerKey{}).(Caller)
	return caller, ok
}

// anonymousKey is the private context key marking a request admitted through
// the static anonymous allowlist (the register_client seam, design.md §1.1).
type anonymousKey struct{}

// MarkAnonymous returns a context marked as an anonymous allowlisted request.
// Such a context carries NO Caller with a role: downstream handlers must read
// the identity from the X-Caller-Id header and use IsAnonymous to distinguish
// this deliberate-anonymity case from a missing or invalid caller.
func MarkAnonymous(ctx context.Context) context.Context {
	return context.WithValue(ctx, anonymousKey{}, true)
}

// IsAnonymous reports whether ctx was marked anonymous by the middleware seam.
func IsAnonymous(ctx context.Context) bool {
	marked, ok := ctx.Value(anonymousKey{}).(bool)
	return ok && marked
}

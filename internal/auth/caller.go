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

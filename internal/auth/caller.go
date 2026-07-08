// Package auth provides authentication and authorization primitives for the
// MCP server. This file contains the Caller value type and context propagation
// helpers used by the middleware (PR 2) and by repositories for audit logging.
package auth

import "context"

// Role constants for the authorization model.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleStaff  = "staff"
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

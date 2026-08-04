package entity

import (
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// AccountRole enumerates the elevated roles on the accounts whitelist.
// Type-safe replacement for the previous `string` field to prevent
// magic-string leaks into tool handlers, MCP tools, and logs.
type AccountRole string

const (
	RoleOwner AccountRole = "owner"
	RoleAdmin AccountRole = "admin"
	RoleStaff AccountRole = "staff"
)

// Account represents a row in the accounts table (whitelist for elevated roles).
// ProfessionalID is nil for admin/owner; non-nil only for staff.
type Account struct {
	ID             string
	Role           AccountRole
	DisplayName    string
	ProfessionalID *string // nullable; required when Role == RoleStaff
	Active         bool
	CreatedAt      string // ISO 8601 UTC (e.g. "2026-07-08T14:30:00.000Z")
	UpdatedAt      string // ISO 8601 UTC
}

// IsActive reports whether the account is enabled.
func (a *Account) IsActive() bool {
	return a.Active
}

// HasRole reports whether the account has the given role.
func (a *Account) HasRole(role AccountRole) bool {
	return a.Role == role
}

// validRole checks if the role is one of the three allowed values.
func validRole(role AccountRole) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleStaff
}

// Validate checks business-rule invariants for an account.
// ID must be non-empty, Role must be valid (owner/admin/staff),
// and staff accounts must have a non-empty ProfessionalID.
func (a *Account) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("el id no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	if !validRole(a.Role) {
		return fmt.Errorf("role %q no válido (debe ser owner, admin o staff): %w", a.Role, domain.ErrInvalidInput)
	}
	if a.Role == RoleStaff && (a.ProfessionalID == nil || *a.ProfessionalID == "") {
		return fmt.Errorf("staff requiere professional_id: %w", domain.ErrInvalidInput)
	}
	return nil
}

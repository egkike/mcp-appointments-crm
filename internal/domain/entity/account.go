package entity

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

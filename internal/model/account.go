package model

// Account represents a row in the accounts table (whitelist for elevated roles).
// ProfessionalID is nil for admin/owner; non-nil only for staff.
// IsActive maps to the is_active INTEGER column (0/1) in SQLite.
// CreatedAt and UpdatedAt are ISO 8601 UTC strings with milliseconds.
type Account struct {
	ID             string
	Role           string // "owner" | "admin" | "staff"
	DisplayName    string
	ProfessionalID *string // nullable; required when Role == "staff"
	IsActive       bool
	CreatedAt      string // ISO 8601 UTC (e.g. "2026-07-08T14:30:00.000Z")
	UpdatedAt      string // ISO 8601 UTC
}

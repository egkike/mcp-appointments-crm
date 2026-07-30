package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// AccountsRepo defines the persistence contract for the accounts whitelist.
// Implementations must return domain.ErrNotFound when a lookup by ID misses.
type AccountsRepo interface {
	// FindByID returns the account with the given ID, or domain.ErrNotFound.
	FindByID(ctx context.Context, id string) (*entity.Account, error)

	// Create inserts a new account. Returns domain.ErrConflict on uniqueness
	// violations (e.g. second owner).
	Create(ctx context.Context, a *entity.Account) error

	// GetByRole returns all accounts with the given role.
	// Valid roles: "owner", "admin", "staff".
	GetByRole(ctx context.Context, role string) ([]*entity.Account, error)

	// List returns all accounts.
	List(ctx context.Context) ([]*entity.Account, error)

	// Update replaces an existing account record.
	Update(ctx context.Context, a *entity.Account) error

	// Deactivate sets the account to inactive by ID.
	Deactivate(ctx context.Context, id string) error

	// IsActive reports whether the account exists and is active.
	IsActive(ctx context.Context, id string) (bool, error)

	// ListByProfessional returns all accounts linked to the given professional.
	ListByProfessional(ctx context.Context, professionalID string) ([]*entity.Account, error)
}

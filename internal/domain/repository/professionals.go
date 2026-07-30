package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ProfessionalsRepo defines the persistence contract for Professional aggregates.
// Implementations must return domain.ErrNotFound when a lookup by ID misses.
type ProfessionalsRepo interface {
	// FindByID returns the professional with the given ID, or domain.ErrNotFound.
	FindByID(ctx context.Context, id string) (*entity.Professional, error)

	// FindActive returns all professionals with active status.
	FindActive(ctx context.Context) ([]*entity.Professional, error)

	// Save inserts a new professional record.
	Save(ctx context.Context, p *entity.Professional) error

	// Update replaces an existing professional record.
	Update(ctx context.Context, p *entity.Professional) error
}

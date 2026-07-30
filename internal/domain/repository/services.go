package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ServicesRepo defines the persistence contract for Service aggregates.
// Implementations must return domain.ErrNotFound when a lookup by ID misses.
type ServicesRepo interface {
	// FindByID returns the service with the given ID, or domain.ErrNotFound.
	FindByID(ctx context.Context, id string) (*entity.Service, error)

	// FindActive returns all services currently available for booking.
	FindActive(ctx context.Context) ([]*entity.Service, error)

	// Save inserts a new service record.
	Save(ctx context.Context, s *entity.Service) error

	// Update replaces an existing service record.
	Update(ctx context.Context, s *entity.Service) error

	// Delete removes a service by ID. Returns domain.ErrNotFound if missing.
	Delete(ctx context.Context, id string) error
}

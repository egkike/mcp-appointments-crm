package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// BusinessProfileRepo defines the persistence contract for the singleton
// BusinessProfile aggregate. Implementations must return domain.ErrNotFound
// when the profile has not been created yet.
type BusinessProfileRepo interface {
	// Get returns the singleton business profile, or domain.ErrNotFound.
	Get(ctx context.Context) (*entity.BusinessProfile, error)

	// Update replaces the singleton business profile record.
	Update(ctx context.Context, p *entity.BusinessProfile) error
}

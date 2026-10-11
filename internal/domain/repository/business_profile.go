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

	// Update replaces the singleton business profile record and returns the row
	// as stored by this call, including the server-side updated_at, so callers
	// never answer with a stale pre-update timestamp. Update can only match zero
	// rows when the singleton row disappears between the read and this write:
	// implementations must return domain.ErrRowGone in that case.
	Update(ctx context.Context, p *entity.BusinessProfile) (*entity.BusinessProfile, error)
}

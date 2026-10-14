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
	//
	// This port is role-generic: implementations re-assert the persistence-layer
	// convention (admin or owner) as a last-resort defense, not the
	// business-scope policy. Caller-scoped narrowing (owner-only for profile
	// maintenance) lives in the use case that owns the decision —
	// update_business_profile enforces auth.RoleOwner (ADR-0015 Decision 2) —
	// and must not be encoded into this adapter, which stays matchable by any
	// legitimate future caller of a wider policy.
	Update(ctx context.Context, p *entity.BusinessProfile) (*entity.BusinessProfile, error)
}

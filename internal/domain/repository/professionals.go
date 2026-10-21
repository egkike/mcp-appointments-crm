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

	// FindActive returns all professionals with active status, scoped to the
	// caller: a staff caller only sees its own professional row.
	FindActive(ctx context.Context) ([]*entity.Professional, error)

	// FindAllActive returns EVERY professional with active status, ordered by
	// name, with NO caller scoping (feat-owner-chat S7a): it is the catalog
	// behind search_professionals_advanced, which any authenticated caller —
	// clients included — may read, so a staff caller must still see the whole
	// roster. Authentication is the use case's responsibility; this method
	// narrows nothing.
	//
	// Contract for implementations: the returned entities still carry email and
	// phone. Callers MUST project them into a PII-free carrier before the data
	// leaves the application layer (ADR-0020 Decision 4).
	FindAllActive(ctx context.Context) ([]*entity.Professional, error)

	// Save inserts a new professional record.
	Save(ctx context.Context, p *entity.Professional) error

	// Update replaces an existing professional record.
	Update(ctx context.Context, p *entity.Professional) error
}

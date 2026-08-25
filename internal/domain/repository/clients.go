package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ClientsRepo defines the persistence contract for Client aggregates.
// Implementations must return domain.ErrNotFound when a lookup misses.
type ClientsRepo interface {
	// FindByID returns the client with the given ID, or domain.ErrNotFound.
	FindByID(ctx context.Context, id string) (*entity.Client, error)

	// FindByPhone returns the client with the given phone number, or domain.ErrNotFound.
	FindByPhone(ctx context.Context, phone string) (*entity.Client, error)

	// Save inserts or updates a client (upsert by ID).
	Save(ctx context.Context, c *entity.Client) error

	// SearchFTS performs a full-text search on clients using FTS5 MATCH.
	// Results are ordered by FTS5 rank (bm25 ASC). Returns
	// domain.ErrInvalidInput if the query is empty or contains FTS5 operators.
	SearchFTS(ctx context.Context, query string) ([]*entity.Client, error)
}

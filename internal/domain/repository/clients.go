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

	// GetOrCreateByPhone returns the client whose id equals phone, creating it
	// when absent. The returned bool reports whether this call created the row
	// (derived from the insert, never from a prior read).
	//
	// Invariant: the stored id MUST be byte-identical to phone — no
	// normalization, no added or removed characters — because CallerResolver
	// matches clients.id == phone; the phone UNIQUE constraint is the hard
	// backstop and an existing row is returned unchanged (its name is never
	// overwritten).
	//
	// The UUID path of the concrete (*ClientsRepo).GetOrCreate is NOT used
	// here (that method is not part of this interface): a generated id can
	// never be resolved again from a phone. This method performs no role
	// check (registration calls it before any caller is resolved); its guards —
	// accounts collision and per-phone rate limit — live in the registration
	// use case.
	//
	// Preconditions: phone and displayName MUST be non-empty and already pass
	// registration-path validation (the use case owns them; an empty phone
	// would insert id == "").
	//
	// Legacy-row edge: a pre-existing row with the same phone but a different
	// id (created by the legacy UUID path) makes INSERT OR IGNORE a no-op and
	// the id read-back miss (ErrNoRows → internal semantic error). Resolving
	// that case is a Phase-2 use-case decision, not adapter behavior.
	//
	// If ctx is cancelled around COMMIT the call may report failure while the
	// row exists; the method is idempotent, so a retry is safe.
	GetOrCreateByPhone(ctx context.Context, phone, displayName string) (entity.Client, bool, error)
}

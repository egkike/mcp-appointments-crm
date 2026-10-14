package repository

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// BusinessHoursExceptionRepo defines the persistence contract for date-specific
// business-hours overrides (holidays, vacations, special events).
type BusinessHoursExceptionRepo interface {
	// Get returns the exception for the given date, or domain.ErrNotFound.
	// The time component of date is ignored; only the calendar date matters.
	Get(ctx context.Context, date time.Time) (*entity.BusinessHoursException, error)

	// Create persists a new business-hours exception and returns the stored
	// row in the same statement: the INSERT is a single atomic
	// `INSERT ... RETURNING` (the PR #114 pattern), so callers get the
	// server-assigned id/created_at without a second read. This closes the
	// read-back TOCTOU window a separate Get would reopen (<R3-readback-window>:
	// a concurrent delete between write and read surfaced as a generic -32603).
	Create(ctx context.Context, e *entity.BusinessHoursException) (*entity.BusinessHoursException, error)

	// List returns all exceptions within the [from, to] date range (inclusive).
	List(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error)

	// Delete removes an exception by its ID. Returns domain.ErrNotFound if missing.
	Delete(ctx context.Context, id int) error
}

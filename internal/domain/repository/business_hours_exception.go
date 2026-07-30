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

	// Create persists a new business-hours exception.
	Create(ctx context.Context, e *entity.BusinessHoursException) error

	// List returns all exceptions within the [from, to] date range (inclusive).
	List(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error)

	// Delete removes an exception by its ID. Returns domain.ErrNotFound if missing.
	Delete(ctx context.Context, id int) error
}

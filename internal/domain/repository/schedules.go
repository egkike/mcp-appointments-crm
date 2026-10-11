package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// SchedulesRepo defines the persistence contract for professional weekly schedules.
type SchedulesRepo interface {
	// FindByProfessionalAndDay returns the schedule for a professional on the
	// given day of week using Go's time.Weekday convention: 0=Sunday,
	// 1=Monday, ..., 6=Saturday. The implementation MUST reject values
	// outside 0-6. Returns domain.ErrNotFound when no schedule exists.
	FindByProfessionalAndDay(ctx context.Context, professionalID string, day int) (*entity.Schedule, error)

	// Upsert inserts or replaces the schedule for the professional+day combination
	// and returns the row as persisted by this call.
	//
	// The adapter MUST read the stored row back inside the same write (a RETURNING
	// clause on the INSERT/UPDATE statement, or a SELECT scoped to the same
	// transaction when the backend cannot return it), so the returned row is
	// exactly what THIS call wrote. A separate, non-transactional read is not a
	// valid implementation: a concurrent upsert landing between the write and that
	// read would make the caller answer with another writer's row.
	Upsert(ctx context.Context, s *entity.Schedule) (*entity.Schedule, error)

	// Delete removes the schedule for a professional on the given day of week.
	Delete(ctx context.Context, professionalID string, day int) error
}

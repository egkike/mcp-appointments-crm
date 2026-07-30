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

	// Upsert inserts or replaces the schedule for the professional+day combination.
	Upsert(ctx context.Context, s *entity.Schedule) error

	// Delete removes the schedule for a professional on the given day of week.
	Delete(ctx context.Context, professionalID string, day int) error
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.SchedulesRepo = (*SchedulesRepo)(nil)

// SchedulesRepo provides CRUD operations for the schedules table.
// Schedules represent a professional's working hours for a specific day of the week.
type SchedulesRepo struct {
	db *sql.DB
}

// NewSchedulesRepo creates a new SchedulesRepo.
func NewSchedulesRepo(db *sql.DB) *SchedulesRepo {
	return &SchedulesRepo{db: db}
}

// validateDayOfWeek checks that day is in the range 0-6 (Sunday-Saturday).
func validateDayOfWeek(day int) error {
	if day < 0 || day > 6 {
		return fmt.Errorf("day_of_week debe estar entre 0 (Domingo) y 6 (Sábado), se recibió %d: %w", day, domain.ErrInvalidInput)
	}
	return nil
}

// validateScheduleTimes checks that start_time and end_time are valid HH:MM format
// and that start_time is strictly before end_time. The format gate is
// entity.ValidHHMM, the single source of truth shared with the config loader and
// the availability service, so the repository path cannot drift from it.
func validateScheduleTimes(startTime, endTime string) error {
	if !entity.ValidHHMM(startTime) {
		return fmt.Errorf("start_time %q no tiene formato HH:MM válido: %w", startTime, domain.ErrInvalidInput)
	}
	if !entity.ValidHHMM(endTime) {
		return fmt.Errorf("end_time %q no tiene formato HH:MM válido: %w", endTime, domain.ErrInvalidInput)
	}
	if startTime >= endTime {
		return fmt.Errorf("start_time (%s) debe ser anterior a end_time (%s): %w", startTime, endTime, domain.ErrInvalidInput)
	}
	return nil
}

// FindByProfessionalAndDay returns the schedule for a professional on a specific day.
// Returns domain.ErrNotFound if no schedule exists for that combination.
// Returns domain.ErrInvalidInput if day_of_week is out of range (0-6).
// Staff callers can only query their own schedule; the professionalID parameter
// is overridden with caller.ProfessionalID.
func (r *SchedulesRepo) FindByProfessionalAndDay(ctx context.Context, professionalID string, dayOfWeek int) (*entity.Schedule, error) {
	if err := validateDayOfWeek(dayOfWeek); err != nil {
		return nil, fmt.Errorf("obtener horario: %w", err)
	}

	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener horario: %w", err)
	}
	// Staff restricted to own schedule — fail-secure if ProfessionalID is nil
	if caller.Role == auth.RoleStaff {
		if caller.ProfessionalID == nil {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeUnauthenticated,
				Message: "Profesional no tiene ID asignado",
				Cause:   domain.ErrUnauthenticated,
			}
		}
		professionalID = *caller.ProfessionalID
	}

	s := &entity.Schedule{}
	err = r.db.QueryRowContext(ctx,
		`SELECT id, professional_id, day_of_week, start_time, end_time
		 FROM schedules WHERE professional_id = ? AND day_of_week = ?`,
		professionalID, dayOfWeek,
	).Scan(&s.ID, &s.ProfessionalID, &s.DayOfWeek, &s.StartTime, &s.EndTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener horario para profesional %s día %d: %w", professionalID, dayOfWeek, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("obtener horario: %w", err)
	}
	return s, nil
}

// scheduleReturningColumns is the read-back projection shared by both upsert
// branches, so the INSERT and UPDATE paths always scan the same shape.
const scheduleReturningColumns = ` RETURNING id, professional_id, day_of_week, start_time, end_time`

// scanStoredSchedule scans the row produced by scheduleReturningColumns.
func scanStoredSchedule(row *sql.Row) (*entity.Schedule, error) {
	stored := &entity.Schedule{}
	if err := row.Scan(&stored.ID, &stored.ProfessionalID, &stored.DayOfWeek, &stored.StartTime, &stored.EndTime); err != nil {
		return nil, err
	}
	return stored, nil
}

// Upsert inserts or updates a schedule. If a schedule already exists for the
// (professional_id, day_of_week) combination, it updates the times; otherwise
// it inserts a new row. It returns the row as written by this call.
// Returns domain.ErrInvalidInput if day_of_week is out of range or times are invalid.
// Returns domain.ErrConflict if professional_id has no matching row in
// professionals (schema.go, schedules.professional_id foreign key).
// Returns domain.ErrNotFound if the matched row disappears before the update lands.
// Requires admin or owner role.
//
// Both branches use INSERT/UPDATE ... RETURNING, a single atomic statement, so
// the row returned is the one this call wrote. That closes the write/read race
// the previous implementation had: it wrote with Exec and the use case then
// issued a separate FindByProfessionalAndDay, which could observe a concurrent
// upsert instead of this one.
func (r *SchedulesRepo) Upsert(ctx context.Context, s *entity.Schedule) (*entity.Schedule, error) {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return nil, fmt.Errorf("upsert horario: %w", err)
	}

	if err := validateDayOfWeek(s.DayOfWeek); err != nil {
		return nil, fmt.Errorf("upsert horario: %w", err)
	}
	if err := validateScheduleTimes(s.StartTime, s.EndTime); err != nil {
		return nil, fmt.Errorf("upsert horario: %w", err)
	}

	// Try INSERT first
	stored, err := scanStoredSchedule(r.db.QueryRowContext(ctx,
		`INSERT INTO schedules (professional_id, day_of_week, start_time, end_time)
		 VALUES (?, ?, ?, ?)`+scheduleReturningColumns,
		s.ProfessionalID, s.DayOfWeek, s.StartTime, s.EndTime,
	))
	if err == nil {
		return stored, nil
	}

	// If UNIQUE constraint violation, UPDATE instead
	if isUniqueViolation(err) {
		stored, updateErr := scanStoredSchedule(r.db.QueryRowContext(ctx,
			`UPDATE schedules SET start_time = ?, end_time = ?
			 WHERE professional_id = ? AND day_of_week = ?`+scheduleReturningColumns,
			s.StartTime, s.EndTime, s.ProfessionalID, s.DayOfWeek,
		))
		if updateErr != nil {
			if errors.Is(updateErr, sql.ErrNoRows) {
				return nil, fmt.Errorf("upsert horario: profesional %s día %d no encontrado: %w",
					s.ProfessionalID, s.DayOfWeek, domain.ErrNotFound)
			}
			return nil, fmt.Errorf("upsert horario: actualizar: %w", updateErr)
		}
		return stored, nil
	}

	// Not a duplicate key: the only other constraint on this INSERT is the
	// schedules.professional_id foreign key (schema.go), so a slot for a
	// professional that does not exist lands here. Translate it so the use case
	// can answer with a semantic error instead of leaking an internal one.
	return nil, fmt.Errorf("upsert horario: %w", classifyForeignKeyViolation(err))
}

// Delete removes a schedule for a professional on a specific day.
// Returns domain.ErrNotFound if no schedule exists for that combination.
// Returns domain.ErrInvalidInput if day_of_week is out of range.
// Requires admin or owner role.
func (r *SchedulesRepo) Delete(ctx context.Context, professionalID string, dayOfWeek int) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("eliminar horario: %w", err)
	}

	if err := validateDayOfWeek(dayOfWeek); err != nil {
		return fmt.Errorf("eliminar horario: %w", err)
	}

	result, err := r.db.ExecContext(ctx,
		`DELETE FROM schedules WHERE professional_id = ? AND day_of_week = ?`,
		professionalID, dayOfWeek,
	)
	if err != nil {
		return fmt.Errorf("eliminar horario: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("eliminar horario: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("eliminar horario: profesional %s día %d: %w", professionalID, dayOfWeek, domain.ErrNotFound)
	}
	return nil
}

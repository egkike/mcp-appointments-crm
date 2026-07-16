package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

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
		return fmt.Errorf("day_of_week debe estar entre 0 (Domingo) y 6 (Sábado), se recibió %d: %w", day, ErrInvalidInput)
	}
	return nil
}

// validateScheduleTimes checks that start_time and end_time are valid HH:MM format
// and that start_time is strictly before end_time.
func validateScheduleTimes(startTime, endTime string) error {
	if !timeHHMMRe.MatchString(startTime) {
		return fmt.Errorf("start_time %q no tiene formato HH:MM válido: %w", startTime, ErrInvalidInput)
	}
	if !timeHHMMRe.MatchString(endTime) {
		return fmt.Errorf("end_time %q no tiene formato HH:MM válido: %w", endTime, ErrInvalidInput)
	}
	if startTime >= endTime {
		return fmt.Errorf("start_time (%s) debe ser anterior a end_time (%s): %w", startTime, endTime, ErrInvalidInput)
	}
	return nil
}

// GetByProfessionalAndDay returns the schedule for a professional on a specific day.
// Returns ErrNotFound if no schedule exists for that combination.
// Returns ErrInvalidInput if day_of_week is out of range (0-6).
// Staff callers can only query their own schedule; the professionalID parameter
// is overridden with caller.ProfessionalID.
func (r *SchedulesRepo) GetByProfessionalAndDay(ctx context.Context, professionalID string, dayOfWeek int) (*model.Schedule, error) {
	if err := validateDayOfWeek(dayOfWeek); err != nil {
		return nil, fmt.Errorf("obtener horario: %w", err)
	}

	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener horario: %w", err)
	}
	// Staff restricted to own schedule — fail-secure if ProfessionalID is nil
	if caller.Role == auth.RoleStaff {
		if caller.ProfessionalID == nil {
			return nil, &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el profesional no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		professionalID = *caller.ProfessionalID
	}

	s := &model.Schedule{}
	err = r.db.QueryRowContext(ctx,
		`SELECT id, professional_id, day_of_week, start_time, end_time
		 FROM schedules WHERE professional_id = ? AND day_of_week = ?`,
		professionalID, dayOfWeek,
	).Scan(&s.ID, &s.ProfessionalID, &s.DayOfWeek, &s.StartTime, &s.EndTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener horario para profesional %s día %d: %w", professionalID, dayOfWeek, ErrNotFound)
		}
		return nil, fmt.Errorf("obtener horario: %w", err)
	}
	return s, nil
}

// Upsert inserts or updates a schedule. If a schedule already exists for the
// (professional_id, day_of_week) combination, it updates the times; otherwise
// it inserts a new row.
// Returns ErrInvalidInput if day_of_week is out of range or times are invalid.
// Requires admin or owner role.
func (r *SchedulesRepo) Upsert(ctx context.Context, s *model.Schedule) error {
	if _, err := requireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("upsert horario: %w", err)
	}

	if err := validateDayOfWeek(s.DayOfWeek); err != nil {
		return fmt.Errorf("upsert horario: %w", err)
	}
	if err := validateScheduleTimes(s.StartTime, s.EndTime); err != nil {
		return fmt.Errorf("upsert horario: %w", err)
	}

	// Try INSERT first
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO schedules (professional_id, day_of_week, start_time, end_time)
		 VALUES (?, ?, ?, ?)`,
		s.ProfessionalID, s.DayOfWeek, s.StartTime, s.EndTime,
	)
	if err == nil {
		return nil
	}

	// If UNIQUE constraint violation, UPDATE instead
	if isUniqueViolation(err) {
		result, updateErr := r.db.ExecContext(ctx,
			`UPDATE schedules SET start_time = ?, end_time = ?
			 WHERE professional_id = ? AND day_of_week = ?`,
			s.StartTime, s.EndTime, s.ProfessionalID, s.DayOfWeek,
		)
		if updateErr != nil {
			return fmt.Errorf("upsert horario: actualizar: %w", updateErr)
		}
		n, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("upsert horario: filas afectadas: %w", rowsErr)
		}
		if n == 0 {
			return fmt.Errorf("upsert horario: profesional %s día %d no encontrado: %w",
				s.ProfessionalID, s.DayOfWeek, ErrNotFound)
		}
		return nil
	}

	return fmt.Errorf("upsert horario: %w", err)
}

// Delete removes a schedule for a professional on a specific day.
// Returns ErrNotFound if no schedule exists for that combination.
// Returns ErrInvalidInput if day_of_week is out of range.
// Requires admin or owner role.
func (r *SchedulesRepo) Delete(ctx context.Context, professionalID string, dayOfWeek int) error {
	if _, err := requireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
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
		return fmt.Errorf("eliminar horario: profesional %s día %d: %w", professionalID, dayOfWeek, ErrNotFound)
	}
	return nil
}

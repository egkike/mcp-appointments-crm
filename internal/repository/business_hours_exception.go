package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// BusinessHoursExceptionRepo provides CRUD for the business_hours_exception
// table. Validates date format and time consistency before hitting the DB.
// Update is intentionally not provided; the only way to change an exception
// is Delete + Create (exceptions are immutable by design).
type BusinessHoursExceptionRepo struct {
	db *sql.DB
}

// NewBusinessHoursExceptionRepo creates a new BusinessHoursExceptionRepo.
func NewBusinessHoursExceptionRepo(db *sql.DB) *BusinessHoursExceptionRepo {
	return &BusinessHoursExceptionRepo{db: db}
}

// Create inserts a new exception. Validates:
//   - exception_date is YYYY-MM-DD (no time component)
//   - is_closed=true requires open_time and close_time to be nil
//   - is_closed=false requires both open_time and close_time in HH:MM format
//   - open_time must be < close_time
//
// Returns ErrInvalidInput for validation failures, ErrConflict for duplicate dates.
func (r *BusinessHoursExceptionRepo) Create(ctx context.Context, ex *model.BusinessHoursException) error {
	// Validate date format and calendar validity via shared helper.
	if err := validateExceptionDate(ex.ExceptionDate); err != nil {
		return fmt.Errorf("crear excepción: %w", err)
	}

	if ex.IsClosed {
		// If closed, open_time and close_time must not be set.
		if ex.OpenTime != nil || ex.CloseTime != nil {
			return fmt.Errorf("crear excepción: si está cerrado, no se deben especificar horarios: %w",
				ErrInvalidInput)
		}
	} else {
		// If open, both times are required.
		if ex.OpenTime == nil || ex.CloseTime == nil {
			return fmt.Errorf("crear excepción: si está abierto, se deben especificar hora de apertura y cierre: %w",
				ErrInvalidInput)
		}
		// Validate HH:MM format.
		if !timeHHMMRe.MatchString(*ex.OpenTime) {
			return fmt.Errorf("crear excepción: la hora de apertura debe tener formato HH:MM, se recibió: %q: %w",
				*ex.OpenTime, ErrInvalidInput)
		}
		if !timeHHMMRe.MatchString(*ex.CloseTime) {
			return fmt.Errorf("crear excepción: la hora de cierre debe tener formato HH:MM, se recibió: %q: %w",
				*ex.CloseTime, ErrInvalidInput)
		}
		// Validate open < close using string comparison (HH:MM is lexicographically ordered).
		if *ex.OpenTime >= *ex.CloseTime {
			return fmt.Errorf("crear excepción: la hora de apertura (%s) debe ser anterior a la hora de cierre (%s): %w",
				*ex.OpenTime, *ex.CloseTime, ErrInvalidInput)
		}
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO business_hours_exception (exception_date, is_closed, open_time, close_time, reason)
		 VALUES (?, ?, ?, ?, ?)`,
		ex.ExceptionDate, ex.IsClosed, ex.OpenTime, ex.CloseTime, ex.Reason,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("crear excepción: la fecha %s ya existe: %w", ex.ExceptionDate, ErrConflict)
		}
		return fmt.Errorf("crear excepción: %w", err)
	}
	return nil
}

// GetByDate returns the exception for a given date. Returns ErrNotFound if
// no exception exists for that date. Validates date format before querying.
func (r *BusinessHoursExceptionRepo) GetByDate(ctx context.Context, date string) (*model.BusinessHoursException, error) {
	if err := validateExceptionDate(date); err != nil {
		return nil, fmt.Errorf("obtener excepción por fecha: %w", err)
	}
	ex := &model.BusinessHoursException{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, exception_date, is_closed, open_time, close_time, reason, created_at
		 FROM business_hours_exception WHERE exception_date = ?`, date,
	).Scan(&ex.ID, &ex.ExceptionDate, &ex.IsClosed, &ex.OpenTime, &ex.CloseTime,
		&ex.Reason, &ex.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener excepción por fecha %s: %w", date, ErrNotFound)
		}
		return nil, fmt.Errorf("obtener excepción por fecha %s: %w", date, err)
	}
	return ex, nil
}

// List returns all exceptions ordered by exception_date ascending.
func (r *BusinessHoursExceptionRepo) List(ctx context.Context) ([]*model.BusinessHoursException, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, exception_date, is_closed, open_time, close_time, reason, created_at
		 FROM business_hours_exception ORDER BY exception_date`)
	if err != nil {
		return nil, fmt.Errorf("listar excepciones: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var exceptions []*model.BusinessHoursException
	for rows.Next() {
		ex := &model.BusinessHoursException{}
		if err := rows.Scan(&ex.ID, &ex.ExceptionDate, &ex.IsClosed, &ex.OpenTime,
			&ex.CloseTime, &ex.Reason, &ex.CreatedAt); err != nil {
			return nil, fmt.Errorf("listar excepciones: escaneo: %w", err)
		}
		exceptions = append(exceptions, ex)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar excepciones: iteración: %w", err)
	}
	return exceptions, nil
}

// Delete removes an exception by ID. Returns ErrNotFound if no row matches.
func (r *BusinessHoursExceptionRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM business_hours_exception WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("eliminar excepción: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("eliminar excepción: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("eliminar excepción: %w", ErrNotFound)
	}
	return nil
}

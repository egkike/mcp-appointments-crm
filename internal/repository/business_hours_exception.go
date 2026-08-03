package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.BusinessHoursExceptionRepo = (*BusinessHoursExceptionRepo)(nil)

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
// Returns domain.ErrInvalidInput for validation failures, domain.ErrConflict for duplicate dates.
func (r *BusinessHoursExceptionRepo) Create(ctx context.Context, ex *entity.BusinessHoursException) error {
	// Validate date format and calendar validity via shared helper.
	if err := validateExceptionDate(ex.ExceptionDate); err != nil {
		return fmt.Errorf("crear excepción: %w", err)
	}

	if ex.IsClosed {
		// If closed, open_time and close_time must not be set.
		if ex.OpenTime != nil || ex.CloseTime != nil {
			return fmt.Errorf("crear excepción: si está cerrado, no se deben especificar horarios: %w",
				domain.ErrInvalidInput)
		}
	} else {
		// If open, both times are required.
		if ex.OpenTime == nil || ex.CloseTime == nil {
			return fmt.Errorf("crear excepción: si está abierto, se deben especificar hora de apertura y cierre: %w",
				domain.ErrInvalidInput)
		}
		// Validate HH:MM format.
		if !timeHHMMRe.MatchString(*ex.OpenTime) {
			return fmt.Errorf("crear excepción: la hora de apertura debe tener formato HH:MM, se recibió: %q: %w",
				*ex.OpenTime, domain.ErrInvalidInput)
		}
		if !timeHHMMRe.MatchString(*ex.CloseTime) {
			return fmt.Errorf("crear excepción: la hora de cierre debe tener formato HH:MM, se recibió: %q: %w",
				*ex.CloseTime, domain.ErrInvalidInput)
		}
		// Validate open < close using string comparison (HH:MM is lexicographically ordered).
		if *ex.OpenTime >= *ex.CloseTime {
			return fmt.Errorf("crear excepción: la hora de apertura (%s) debe ser anterior a la hora de cierre (%s): %w",
				*ex.OpenTime, *ex.CloseTime, domain.ErrInvalidInput)
		}
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO business_hours_exception (exception_date, is_closed, open_time, close_time, reason)
		 VALUES (?, ?, ?, ?, ?)`,
		ex.ExceptionDate, ex.IsClosed, ex.OpenTime, ex.CloseTime, ex.Reason,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("crear excepción: la fecha %s ya existe: %w", ex.ExceptionDate, domain.ErrConflict)
		}
		return fmt.Errorf("crear excepción: %w", err)
	}
	return nil
}

// Get returns the exception for a given date. Returns domain.ErrNotFound if
// no exception exists for that date. The time component of date is ignored;
// only the calendar date matters.
func (r *BusinessHoursExceptionRepo) Get(ctx context.Context, date time.Time) (*entity.BusinessHoursException, error) {
	dateStr := date.Format("2006-01-02")
	ex := &entity.BusinessHoursException{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, exception_date, is_closed, open_time, close_time, reason, created_at
		 FROM business_hours_exception WHERE exception_date = ?`, dateStr,
	).Scan(&ex.ID, &ex.ExceptionDate, &ex.IsClosed, &ex.OpenTime, &ex.CloseTime,
		&ex.Reason, &ex.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener excepción por fecha %s: %w", dateStr, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("obtener excepción por fecha %s: %w", dateStr, err)
	}
	return ex, nil
}

// List returns all exceptions within the [from, to] date range (inclusive),
// ordered by exception_date ascending.
func (r *BusinessHoursExceptionRepo) List(ctx context.Context, from, to time.Time) ([]*entity.BusinessHoursException, error) {
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, exception_date, is_closed, open_time, close_time, reason, created_at
		 FROM business_hours_exception
		 WHERE exception_date >= ? AND exception_date <= ?
		 ORDER BY exception_date`,
		fromStr, toStr,
	)
	if err != nil {
		return nil, fmt.Errorf("listar excepciones: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var exceptions []*entity.BusinessHoursException
	for rows.Next() {
		ex := &entity.BusinessHoursException{}
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

// Delete removes an exception by ID. Returns domain.ErrNotFound if no row matches.
func (r *BusinessHoursExceptionRepo) Delete(ctx context.Context, id int) error {
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
		return fmt.Errorf("eliminar excepción: %w", domain.ErrNotFound)
	}
	return nil
}

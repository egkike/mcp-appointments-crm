package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// BusinessHoursExceptionRepo provides CRUD for the business_hours_exception
// table. Validates date format and time consistency before hitting the DB.
type BusinessHoursExceptionRepo struct {
	db *sql.DB
}

// NewBusinessHoursExceptionRepo creates a new BusinessHoursExceptionRepo.
func NewBusinessHoursExceptionRepo(db *sql.DB) *BusinessHoursExceptionRepo {
	return &BusinessHoursExceptionRepo{db: db}
}

// datePattern matches YYYY-MM-DD strictly (no time component).
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Create inserts a new exception. Validates:
//   - exception_date is YYYY-MM-DD (no time component)
//   - is_closed=false requires both open_time and close_time
//   - open_time must be < close_time
//
// Returns ErrInvalidInput for validation failures, ErrConflict for duplicate dates.
func (r *BusinessHoursExceptionRepo) Create(ctx context.Context, ex *model.BusinessHoursException) error {
	// Validate date format.
	if !datePattern.MatchString(ex.ExceptionDate) {
		return fmt.Errorf("create exception: exception_date must be YYYY-MM-DD, got %q: %w",
			ex.ExceptionDate, ErrInvalidInput)
	}

	// Validate time consistency when not closed.
	if !ex.IsClosed {
		if ex.OpenTime == nil || ex.CloseTime == nil {
			return fmt.Errorf("create exception: is_closed=false requires both open_time and close_time: %w",
				ErrInvalidInput)
		}
		if *ex.OpenTime >= *ex.CloseTime {
			return fmt.Errorf("create exception: open_time (%s) must be before close_time (%s): %w",
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
			return fmt.Errorf("create exception: date %s already exists: %w", ex.ExceptionDate, ErrConflict)
		}
		return fmt.Errorf("create exception: %w", err)
	}
	return nil
}

// GetByDate returns the exception for a given date. Returns ErrNotFound if
// no exception exists for that date.
func (r *BusinessHoursExceptionRepo) GetByDate(ctx context.Context, date string) (*model.BusinessHoursException, error) {
	ex := &model.BusinessHoursException{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, exception_date, is_closed, open_time, close_time, reason, created_at
		 FROM business_hours_exception WHERE exception_date = ?`, date,
	).Scan(&ex.ID, &ex.ExceptionDate, &ex.IsClosed, &ex.OpenTime, &ex.CloseTime,
		&ex.Reason, &ex.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get exception by date %s: %w", date, ErrNotFound)
		}
		return nil, fmt.Errorf("get exception by date %s: %w", date, err)
	}
	return ex, nil
}

// List returns all exceptions ordered by exception_date ascending.
func (r *BusinessHoursExceptionRepo) List(ctx context.Context) ([]*model.BusinessHoursException, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, exception_date, is_closed, open_time, close_time, reason, created_at
		 FROM business_hours_exception ORDER BY exception_date`)
	if err != nil {
		return nil, fmt.Errorf("list exceptions: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var exceptions []*model.BusinessHoursException
	for rows.Next() {
		ex := &model.BusinessHoursException{}
		if err := rows.Scan(&ex.ID, &ex.ExceptionDate, &ex.IsClosed, &ex.OpenTime,
			&ex.CloseTime, &ex.Reason, &ex.CreatedAt); err != nil {
			return nil, fmt.Errorf("list exceptions: scan: %w", err)
		}
		exceptions = append(exceptions, ex)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list exceptions: rows: %w", err)
	}
	return exceptions, nil
}

// Delete removes an exception by ID. Returns ErrNotFound if no row matches.
func (r *BusinessHoursExceptionRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM business_hours_exception WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete exception: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete exception rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("delete exception: %w", ErrNotFound)
	}
	return nil
}

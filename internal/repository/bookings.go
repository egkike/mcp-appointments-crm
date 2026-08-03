package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.BookingsRepo = (*BookingsRepo)(nil)

// BookingsRepo provides CRUD operations for the bookings table.
// Create and Reschedule use atomic INSERT/UPDATE ... WHERE NOT EXISTS overlap checks
// (per design Decisión 11).
type BookingsRepo struct {
	db *sql.DB
}

// NewBookingsRepo creates a new BookingsRepo.
func NewBookingsRepo(db *sql.DB) *BookingsRepo {
	return &BookingsRepo{db: db}
}

// parseStorageTime parses a storage-format datetime string back to time.Time (UTC).
func parseStorageTime(s string) (time.Time, error) {
	t, err := time.Parse(storageTimeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse storage datetime %q: %w", s, err)
	}
	return t, nil
}

// scanBooking scans a full booking row into an entity.Booking.
// The caller must provide columns in the canonical order:
// id, client_id, professional_id, service_id, start_datetime, end_datetime,
// status, notes, payment_method, created_at, updated_at.
func scanBooking(scan func(dest ...any) error) (*entity.Booking, error) {
	var b entity.Booking
	var startStr, endStr, createdAtStr, updatedAtStr string
	if err := scan(
		&b.ID, &b.ClientID, &b.ProfessionalID, &b.ServiceID,
		&startStr, &endStr, (*string)(&b.Status), &b.Notes, &b.PaymentMethod,
		&createdAtStr, &updatedAtStr,
	); err != nil {
		return nil, err
	}
	var err error
	if b.StartDatetime, err = parseStorageTime(startStr); err != nil {
		return nil, fmt.Errorf("scan start_datetime: %w", err)
	}
	if b.EndDatetime, err = parseStorageTime(endStr); err != nil {
		return nil, fmt.Errorf("scan end_datetime: %w", err)
	}
	if b.CreatedAt, err = parseStorageTime(createdAtStr); err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	if b.UpdatedAt, err = parseStorageTime(updatedAtStr); err != nil {
		return nil, fmt.Errorf("scan updated_at: %w", err)
	}
	return &b, nil
}

// bookingColumns is the canonical SELECT column list for booking queries.
const bookingColumns = `id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes, payment_method, created_at, updated_at`

// ─── Domain interface methods ────────────────────────────────────────────────

// Create persists a new booking with an atomic overlap check.
//
// The caller (use case) is responsible for:
//   - Generating the booking ID
//   - Computing EndDatetime from service duration
//   - Setting Status to entity.BookingStatusPending
//   - Authorization (RequireClientMatch / role checks)
//
// The method executes an atomic INSERT ... WHERE NOT EXISTS (overlap subquery).
// If RowsAffected() == 0, returns an error wrapping domain.ErrConflict.
func (r *BookingsRepo) Create(ctx context.Context, b *entity.Booking) error {
	startStr := FormatStorage(b.StartDatetime)
	endStr := FormatStorage(b.EndDatetime)

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes, payment_method, created_at, updated_at)
		 SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE NOT EXISTS (
		     SELECT 1 FROM bookings
		     WHERE professional_id = ? AND status != 'cancelled'
		       AND start_datetime < ? AND end_datetime > ?
		 )`,
		b.ID, b.ClientID, b.ProfessionalID, b.ServiceID,
		startStr, endStr, string(b.Status), b.Notes, b.PaymentMethod,
		b.ProfessionalID, endStr, startStr,
	)
	if err != nil {
		return fmt.Errorf("crear reserva: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("crear reserva: filas afectadas: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("crear reserva: overlap: %w", domain.ErrConflict)
	}

	return nil
}

// FindByID returns a booking by ID. Returns domain.ErrNotFound if not found or if
// the caller does not have access (unified — no existence oracle).
// Auth: dynamic WHERE filters by caller scope (per design.md §500).
//   - client: WHERE id = ? AND client_id = ?
//   - staff: WHERE id = ? AND professional_id = ?
//   - admin/owner: WHERE id = ?
func (r *BookingsRepo) FindByID(ctx context.Context, id string) (*entity.Booking, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener reserva %s: %w", id, err)
	}

	query := `SELECT ` + bookingColumns + ` FROM bookings WHERE id = ?`
	args := []any{id}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	b, err := scanBooking(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener reserva %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("obtener reserva %s: %w", id, err)
	}

	return b, nil
}

// Update replaces the booking record. The booking must already exist.
// Auth filter applies: client/staff callers can only update their own bookings.
// Returns domain.ErrNotFound if no rows are affected.
func (r *BookingsRepo) Update(ctx context.Context, b *entity.Booking) error {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return fmt.Errorf("actualizar reserva %s: %w", b.ID, err)
	}

	startStr := FormatStorage(b.StartDatetime)
	endStr := FormatStorage(b.EndDatetime)

	query := `UPDATE bookings SET client_id=?, professional_id=?, service_id=?, start_datetime=?, end_datetime=?, status=?, notes=?, payment_method=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id=?`
	args := []any{b.ClientID, b.ProfessionalID, b.ServiceID, startStr, endStr, string(b.Status), b.Notes, b.PaymentMethod, b.ID}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return fmt.Errorf("actualizar reserva %s: %w", b.ID, err)
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("actualizar reserva %s: %w", b.ID, err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar reserva %s: filas afectadas: %w", b.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("actualizar reserva %s: %w", b.ID, domain.ErrNotFound)
	}
	return nil
}

// Cancel transitions a booking to "cancelled" status.
// Allowed transitions: pending→cancelled, confirmed→cancelled.
// Returns *domain.SemanticError{Code: domain.ErrCodeInvalidInput} for cancelled→cancelled.
// Returns domain.ErrNotFound if the booking does not exist or caller lacks access.
func (r *BookingsRepo) Cancel(ctx context.Context, id string) error {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return fmt.Errorf("cancelar reserva %s: %w", id, err)
	}

	query := `SELECT status FROM bookings WHERE id = ?`
	args := []any{id}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return err
	}

	var currentStatus entity.BookingStatus
	err = r.db.QueryRowContext(ctx, query, args...).Scan((*string)(&currentStatus))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cancelar reserva %s: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("cancelar reserva %s: consultar estado: %w", id, err)
	}

	// Validate FSM transition: only pending and confirmed can be cancelled.
	if currentStatus != entity.BookingStatusPending && currentStatus != entity.BookingStatusConfirmed {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: fmt.Sprintf("La transición de %q a 'cancelled' no está permitida.", currentStatus),
		}
	}

	// Defense in depth: also apply auth filter to the UPDATE so a race or
	// schema drift cannot allow a cross-tenant cancel between the SELECT and the UPDATE.
	updateQuery := `UPDATE bookings SET status = 'cancelled', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`
	updateArgs := []any{id}
	updateQuery, updateArgs, err = applyAuthFilter(caller, updateQuery, updateArgs)
	if err != nil {
		return fmt.Errorf("cancelar reserva %s: %w", id, err)
	}
	_, err = r.db.ExecContext(ctx, updateQuery, updateArgs...)
	if err != nil {
		return fmt.Errorf("cancelar reserva %s: %w", id, err)
	}
	return nil
}

// Reschedule updates the start and end times of an existing booking.
// Uses an atomic UPDATE ... WHERE NOT EXISTS overlap guard.
// The caller (use case) computes newEnd from service duration.
// Returns domain.ErrConflict if the new slot overlaps.
// Returns domain.ErrNotFound if the booking does not exist or caller lacks access.
func (r *BookingsRepo) Reschedule(ctx context.Context, id string, newStart, newEnd time.Time) error {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: %w", id, err)
	}

	query := `SELECT status, professional_id FROM bookings WHERE id = ?`
	args := []any{id}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return err
	}

	var professionalID string
	var currentStatus entity.BookingStatus
	err = r.db.QueryRowContext(ctx, query, args...).Scan((*string)(&currentStatus), &professionalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reprogramar reserva %s: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("reprogramar reserva %s: consultar: %w", id, err)
	}

	if currentStatus == entity.BookingStatusCancelled {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "No se puede reprogramar una reserva cancelada.",
		}
	}

	startStr := FormatStorage(newStart)
	endStr := FormatStorage(newEnd)

	// Defense in depth: apply auth filter to the UPDATE. For client callers, this
	// scopes the UPDATE to client_id = caller.ClientID. For staff callers, the filter
	// also requires professional_id = caller.ProfessionalID (which combined with the
	// arg's professional_id produces a no-op for the caller's own bookings).
	updateQuery := `UPDATE bookings
		 SET start_datetime = ?, end_datetime = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id = ?
		   AND status != 'cancelled'
		   AND NOT EXISTS (
		     SELECT 1 FROM bookings
		     WHERE id != ?
		       AND professional_id = ?
		       AND status != 'cancelled'
		       AND start_datetime < ?
		       AND end_datetime > ?
		   )`
	updateArgs := []any{startStr, endStr, id, id, professionalID, endStr, startStr}
	updateQuery, updateArgs, err = applyAuthFilter(caller, updateQuery, updateArgs)
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: %w", id, err)
	}

	result, err := r.db.ExecContext(ctx, updateQuery, updateArgs...)
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: filas afectadas: %w", id, err)
	}

	if rowsAffected == 0 {
		// Defense in depth: also apply auth filter to the recheck SELECT.
		recheckQuery := `SELECT status FROM bookings WHERE id = ?`
		recheckArgs := []any{id}
		recheckQuery, recheckArgs, err = applyAuthFilter(caller, recheckQuery, recheckArgs)
		if err != nil {
			return fmt.Errorf("reprogramar reserva %s: %w", id, err)
		}
		var recheckStatus entity.BookingStatus
		err := r.db.QueryRowContext(ctx, recheckQuery, recheckArgs...).
			Scan((*string)(&recheckStatus))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("reprogramar reserva %s: %w", id, domain.ErrNotFound)
			}
			return fmt.Errorf("reprogramar reserva %s: verificar estado: %w", id, err)
		}
		if recheckStatus == entity.BookingStatusCancelled {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "No se puede reprogramar una reserva cancelada",
			}
		}
		return fmt.Errorf("reprogramar reserva %s: overlap: %w", id, domain.ErrConflict)
	}

	return nil
}

// FindOverlapping returns non-cancelled bookings for the given staff member that
// overlap the [start, end) window. Returns an empty slice (not nil) when none match.
// Auth: caller must be authenticated; client callers see only their own bookings,
// staff callers see only their own professional's bookings, admin/owner see all.
func (r *BookingsRepo) FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("buscar reservas superpuestas: %w", err)
	}
	startStr := FormatStorage(start)
	endStr := FormatStorage(end)

	query := `SELECT ` + bookingColumns + ` FROM bookings
		 WHERE professional_id = ? AND status != 'cancelled'
		   AND start_datetime < ? AND end_datetime > ?
		 ORDER BY start_datetime ASC`
	args := []any{staffID, endStr, startStr}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return nil, fmt.Errorf("buscar reservas superpuestas: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("buscar reservas superpuestas: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	bookings := make([]*entity.Booking, 0)
	for rows.Next() {
		b, err := scanBooking(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("buscar reservas superpuestas: escaneo: %w", err)
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar reservas superpuestas: iteración: %w", err)
	}
	return bookings, nil
}

// FindByStaffAndRange returns all non-cancelled bookings for a staff member within
// the [start, end) time range, ordered by start_datetime ASC. Used by calendar views.
// Note: the SQL is intentionally identical to FindOverlapping; the semantic
// distinction is that this returns all bookings in the range, not just overlapping ones.
// Auth: caller must be authenticated; client callers see only their own bookings,
// staff callers see only their own professional's bookings, admin/owner see all.
func (r *BookingsRepo) FindByStaffAndRange(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar reservas por profesional y rango: %w", err)
	}
	startStr := FormatStorage(start)
	endStr := FormatStorage(end)

	query := `SELECT ` + bookingColumns + ` FROM bookings
		 WHERE professional_id = ? AND status != 'cancelled'
		   AND start_datetime < ? AND end_datetime > ?
		 ORDER BY start_datetime ASC`
	args := []any{staffID, endStr, startStr}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return nil, fmt.Errorf("listar reservas por profesional y rango: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listar reservas por profesional y rango: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	bookings := make([]*entity.Booking, 0)
	for rows.Next() {
		b, err := scanBooking(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("listar reservas por profesional y rango: escaneo: %w", err)
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar reservas por profesional y rango: iteración: %w", err)
	}
	return bookings, nil
}

// ListBookingsForRange returns all non-cancelled bookings across all staff within
// the [start, end) time range, ordered by professional_id then start_datetime ASC.
// Used by master calendar views.
// Auth: caller must be authenticated; client callers see only their own bookings,
// staff callers see only their own professional's bookings, admin/owner see all.
func (r *BookingsRepo) ListBookingsForRange(ctx context.Context, start, end time.Time) ([]*entity.Booking, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar reservas por rango: %w", err)
	}
	startStr := FormatStorage(start)
	endStr := FormatStorage(end)

	query := `SELECT ` + bookingColumns + ` FROM bookings
		 WHERE status != 'cancelled'
		   AND start_datetime < ? AND end_datetime > ?
		 ORDER BY professional_id, start_datetime ASC`
	args := []any{endStr, startStr}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return nil, fmt.Errorf("listar reservas por rango: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listar reservas por rango: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	bookings := make([]*entity.Booking, 0)
	for rows.Next() {
		b, err := scanBooking(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("listar reservas por rango: escaneo: %w", err)
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar reservas por rango: iteración: %w", err)
	}
	return bookings, nil
}

// SearchByNotes returns bookings whose notes contain the query substring.
// Limited to 100 results. Cancelled bookings are included.
// Auth: caller must be authenticated; client callers see only their own bookings,
// staff callers see only their own professional's bookings, admin/owner see all.
func (r *BookingsRepo) SearchByNotes(ctx context.Context, q string) ([]*entity.Booking, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("buscar reservas por notas: %w", err)
	}

	query := `SELECT ` + bookingColumns + ` FROM bookings
		 WHERE notes LIKE '%' || ? || '%'
		 ORDER BY start_datetime DESC
		 LIMIT 100`
	args := []any{q}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return nil, fmt.Errorf("buscar reservas por notas: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("buscar reservas por notas: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	bookings := make([]*entity.Booking, 0)
	for rows.Next() {
		b, err := scanBooking(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("buscar reservas por notas: escaneo: %w", err)
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar reservas por notas: iteración: %w", err)
	}
	return bookings, nil
}

// UpdateStatus changes the status of a booking by ID.
// Auth filter applies. Returns domain.ErrNotFound if no rows are affected.
func (r *BookingsRepo) UpdateStatus(ctx context.Context, id string, status entity.BookingStatus) error {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return fmt.Errorf("actualizar estado reserva %s: %w", id, err)
	}

	query := `UPDATE bookings SET status=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id=?`
	args := []any{string(status), id}

	query, args, err = applyAuthFilter(caller, query, args)
	if err != nil {
		return fmt.Errorf("actualizar estado reserva %s: %w", id, err)
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("actualizar estado reserva %s: %w", id, err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar estado reserva %s: filas afectadas: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("actualizar estado reserva %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

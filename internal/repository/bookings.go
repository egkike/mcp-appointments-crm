package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// BookingsRepo provides CRUD operations for the bookings table.
// CreateBooking uses an atomic INSERT ... WHERE NOT EXISTS overlap check
// (per design Decisión 11). CheckAvailability (3a-3c, 3e validations) is
// NOT implemented in this PR — it belongs to PR 3b.
type BookingsRepo struct {
	db *sql.DB
}

// NewBookingsRepo creates a new BookingsRepo.
func NewBookingsRepo(db *sql.DB) *BookingsRepo {
	return &BookingsRepo{db: db}
}

// CreateBookingInput holds the input parameters for CreateBooking.
type CreateBookingInput struct {
	ClientID       string
	ProfessionalID string
	ServiceID      string
	StartDatetime  string // RFC3339 format (e.g., "2026-07-13T13:00:00-03:00" or "2026-07-13T13:00:00.000Z")
	Notes          *string
	PaymentMethod  *string
}

// CreateBookingResult holds the result of CreateBooking.
type CreateBookingResult struct {
	Booking *model.Booking
}

// CreateBooking creates a new booking with an atomic overlap check.
//
// BYPASS ASSUMPTION (per design Decisión 11 + S1): This method does NOT run
// the full check_availability validations (3a business hours, 3b professional
// schedule, 3c slot within hours, 3e not in past). It ONLY runs the atomic
// 3d overlap check via INSERT ... WHERE NOT EXISTS. The caller (MCP handler)
// is expected to call CheckAvailability first for a non-authoritative preview.
//
// The method:
// 1. Queries services.duration_minutes to compute end_datetime
// 2. Parses start_datetime to time.Time (UTC)
// 3. Computes end_datetime = start + duration
// 4. Executes atomic INSERT ... WHERE NOT EXISTS (overlap subquery)
// 5. If RowsAffected() == 0, returns SemanticError{Code: ErrCodeBookingOverlap}
//
// Returns ErrNotFound if the service does not exist.
// Returns ErrInvalidInput if start_datetime is empty or malformed.
// Returns *SemanticError{Code: ErrCodeBookingOverlap} if the slot overlaps.
func (r *BookingsRepo) CreateBooking(ctx context.Context, input *CreateBookingInput) (*CreateBookingResult, error) {
	if strings.TrimSpace(input.StartDatetime) == "" {
		return nil, fmt.Errorf("crear reserva: start_datetime no puede estar vacío: %w", ErrInvalidInput)
	}

	// Auth: client can only create for themselves; staff must match their professional; admin/owner can create for any client
	if err := requireClientMatch(ctx, input.ClientID, input.ProfessionalID); err != nil {
		return nil, fmt.Errorf("crear reserva: %w", err)
	}

	// Query service duration
	var durationMinutes int
	err := r.db.QueryRowContext(ctx,
		`SELECT duration_minutes FROM services WHERE id = ?`, input.ServiceID,
	).Scan(&durationMinutes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("crear reserva: el servicio %s no existe: %w", input.ServiceID, ErrNotFound)
		}
		return nil, fmt.Errorf("crear reserva: consultar servicio: %w", err)
	}

	// Parse start_datetime to time.Time (UTC)
	startTime, err := ParseStartDatetime(input.StartDatetime, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("crear reserva: %w", err)
	}
	startUTC := startTime.UTC()
	endUTC := startUTC.Add(time.Duration(durationMinutes) * time.Minute)

	// Format for storage
	startStr := FormatStorage(startUTC)
	endStr := FormatStorage(endUTC)

	// Generate UUID for booking ID
	bookingID := model.NewUUID()

	// Atomic INSERT with overlap check
	// strftime format must match storageTimeLayout in datetime.go.
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes, payment_method, created_at, updated_at)
		 SELECT ?, ?, ?, ?, ?, ?, 'pending', ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE NOT EXISTS (
		     SELECT 1 FROM bookings
		     WHERE professional_id = ? AND status != 'cancelled'
		       AND start_datetime < ? AND end_datetime > ?
		 )`,
		bookingID, input.ClientID, input.ProfessionalID, input.ServiceID,
		startStr, endStr, input.Notes, input.PaymentMethod,
		// Note: WHERE placeholders read start_datetime < ? AND end_datetime > ? but
		// args are (endStr, startStr) — positional binding means the first ? takes
		// the first arg after the value-args, etc. Order matches the SQL.
		input.ProfessionalID, endStr, startStr,
	)
	if err != nil {
		return nil, fmt.Errorf("crear reserva: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("crear reserva: filas afectadas: %w", err)
	}

	if rowsAffected == 0 {
		return nil, &SemanticError{
			Code:    ErrCodeBookingOverlap,
			Message: fmt.Sprintf("el Profesional %s ya tiene una reserva en ese horario.", input.ProfessionalID),
		}
	}

	// Build result
	booking := &model.Booking{
		ID:             bookingID,
		ClientID:       input.ClientID,
		ProfessionalID: input.ProfessionalID,
		ServiceID:      input.ServiceID,
		StartDatetime:  startStr,
		EndDatetime:    endStr,
		Status:         model.BookingStatusPending,
		Notes:          input.Notes,
		PaymentMethod:  input.PaymentMethod,
	}

	return &CreateBookingResult{Booking: booking}, nil
}

// GetBooking returns a booking by ID. Returns ErrNotFound if not found or if
// the caller does not have access (unified — no existence oracle).
// Auth: dynamic WHERE filters by caller scope (per design.md §500).
//   - client: WHERE id = ? AND client_id = ?
//   - staff: WHERE id = ? AND professional_id = ?
//   - admin/owner: WHERE id = ?
func (r *BookingsRepo) GetBooking(ctx context.Context, id string) (*model.Booking, error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener reserva %s: %w", id, err)
	}

	query := `SELECT id, client_id, professional_id, service_id, start_datetime, end_datetime, status, notes, payment_method, created_at, updated_at
		 FROM bookings WHERE id = ?`
	args := []any{id}

	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil {
			return nil, &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el cliente no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		query += ` AND client_id = ?`
		args = append(args, *caller.ClientID)
	case auth.RoleStaff:
		if caller.ProfessionalID == nil {
			return nil, &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el profesional no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		query += ` AND professional_id = ?`
		args = append(args, *caller.ProfessionalID)
	case auth.RoleAdmin, auth.RoleOwner:
		// no extra filter
	default:
		return nil, &SemanticError{
			Code:    ErrCodeUnauthenticated,
			Message: fmt.Sprintf("el rol %q no tiene permiso para acceder a reservas", caller.Role),
			Cause:   ErrUnauthenticated,
		}
	}

	b := &model.Booking{}
	err = r.db.QueryRowContext(ctx, query, args...).Scan(
		&b.ID, &b.ClientID, &b.ProfessionalID, &b.ServiceID,
		&b.StartDatetime, &b.EndDatetime, &b.Status, &b.Notes, &b.PaymentMethod,
		&b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("obtener reserva %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("obtener reserva %s: %w", id, err)
	}

	return b, nil
}

// CancelBooking transitions a booking to "cancelled" status.
// Allowed transitions: pending→cancelled, confirmed→cancelled.
// Returns *SemanticError{Code: ErrCodeInvalidInput} for cancelled→cancelled.
// Returns ErrNotFound if the booking does not exist.
// Auth: client callers can only cancel their own bookings; staff callers can
// only cancel bookings for their professional; admin/owner see all.
func (r *BookingsRepo) CancelBooking(ctx context.Context, id string) error {
	caller, err := requireCaller(ctx)
	if err != nil {
		return fmt.Errorf("cancelar reserva %s: %w", id, err)
	}

	// Dynamic WHERE: auth filter in query itself (same pattern as GetBooking).
	// Cross-tenant and non-existent rows both return ErrNotFound (no oracle).
	query := `SELECT status FROM bookings WHERE id = ?`
	args := []any{id}

	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil {
			return &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el cliente no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		query += ` AND client_id = ?`
		args = append(args, *caller.ClientID)
	case auth.RoleStaff:
		if caller.ProfessionalID == nil {
			return &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el profesional no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		query += ` AND professional_id = ?`
		args = append(args, *caller.ProfessionalID)
	case auth.RoleAdmin, auth.RoleOwner:
		// no extra filter
	default:
		return &SemanticError{
			Code:    ErrCodeUnauthenticated,
			Message: fmt.Sprintf("el rol %q no tiene permiso para acceder a reservas", caller.Role),
			Cause:   ErrUnauthenticated,
		}
	}

	var currentStatus model.BookingStatus
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cancelar reserva %s: %w", id, ErrNotFound)
		}
		return fmt.Errorf("cancelar reserva %s: consultar estado: %w", id, err)
	}

	// Validate FSM transition
	if !currentStatus.IsValidTransition(model.BookingStatusCancelled) {
		return &SemanticError{
			Code:    ErrCodeInvalidInput,
			Message: fmt.Sprintf("la transición de %q a 'cancelled' no está permitida.", currentStatus),
		}
	}

	// strftime format must match storageTimeLayout in datetime.go.
	_, err = r.db.ExecContext(ctx,
		`UPDATE bookings SET status = 'cancelled', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("cancelar reserva %s: %w", id, err)
	}
	return nil
}

// RescheduleBooking updates the start_datetime of a booking and recomputes end_datetime.
// Uses an atomic UPDATE ... WHERE NOT EXISTS overlap guard (same shape as
// CreateBooking). If RowsAffected() == 0, the new slot overlaps with an
// existing non-cancelled booking for the same professional.
// Returns *SemanticError{Code: ErrCodeBookingOverlap} if the new slot overlaps.
// Returns *SemanticError{Code: ErrCodeInvalidInput} if the booking is cancelled.
// Returns ErrNotFound if the booking does not exist.
// Auth: client callers can only reschedule their own bookings; staff callers can
// only reschedule bookings for their professional; admin/owner see all.
func (r *BookingsRepo) RescheduleBooking(ctx context.Context, id string, newStartDatetime string) error {
	caller, err := requireCaller(ctx)
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: %w", id, err)
	}

	// Dynamic WHERE: auth filter in query itself (same pattern as GetBooking).
	// Cross-tenant and non-existent rows both return ErrNotFound (no oracle).
	query := `SELECT service_id, status, professional_id FROM bookings WHERE id = ?`
	args := []any{id}

	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil {
			return &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el cliente no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		query += ` AND client_id = ?`
		args = append(args, *caller.ClientID)
	case auth.RoleStaff:
		if caller.ProfessionalID == nil {
			return &SemanticError{
				Code:    ErrCodeUnauthenticated,
				Message: "el profesional no tiene ID asignado",
				Cause:   ErrUnauthenticated,
			}
		}
		query += ` AND professional_id = ?`
		args = append(args, *caller.ProfessionalID)
	case auth.RoleAdmin, auth.RoleOwner:
		// no extra filter
	default:
		return &SemanticError{
			Code:    ErrCodeUnauthenticated,
			Message: fmt.Sprintf("el rol %q no tiene permiso para acceder a reservas", caller.Role),
			Cause:   ErrUnauthenticated,
		}
	}

	var serviceID, professionalID string
	var currentStatus model.BookingStatus
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&serviceID, &currentStatus, &professionalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reprogramar reserva %s: %w", id, ErrNotFound)
		}
		return fmt.Errorf("reprogramar reserva %s: consultar: %w", id, err)
	}

	// Cannot reschedule cancelled bookings
	if currentStatus == model.BookingStatusCancelled {
		return &SemanticError{
			Code:    ErrCodeInvalidInput,
			Message: "no se puede reprogramar una reserva cancelada.",
		}
	}

	// Query service duration
	var durationMinutes int
	err = r.db.QueryRowContext(ctx,
		`SELECT duration_minutes FROM services WHERE id = ?`, serviceID,
	).Scan(&durationMinutes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reprogramar reserva %s: el servicio %s no existe: %w", id, serviceID, ErrNotFound)
		}
		return fmt.Errorf("reprogramar reserva %s: consultar servicio: %w", id, err)
	}

	// Parse new start_datetime
	startTime, err := ParseStartDatetime(newStartDatetime, time.UTC)
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: %w", id, err)
	}
	startUTC := startTime.UTC()
	endUTC := startUTC.Add(time.Duration(durationMinutes) * time.Minute)

	startStr := FormatStorage(startUTC)
	endStr := FormatStorage(endUTC)

	// Atomic UPDATE with overlap check (same shape as CreateBooking).
	// The WHERE clause excludes the current booking (id != ?) and checks for
	// other non-cancelled bookings for the same professional that overlap.
	// strftime format must match storageTimeLayout in datetime.go.
	result, err := r.db.ExecContext(ctx,
		`UPDATE bookings
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
		   )`,
		// Note: WHERE placeholders read start_datetime < ? AND end_datetime > ? but
		// args are (endStr, startStr) — positional binding matches the SQL order.
		startStr, endStr, id, id, professionalID, endStr, startStr,
	)
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reprogramar reserva %s: filas afectadas: %w", id, err)
	}

	if rowsAffected == 0 {
		// Re-check the booking status to disambiguate overlap vs concurrent cancellation.
		var recheckStatus model.BookingStatus
		err := r.db.QueryRowContext(ctx,
			`SELECT status FROM bookings WHERE id = ?`, id,
		).Scan(&recheckStatus)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("reprogramar reserva %s: %w", id, ErrNotFound)
			}
			return fmt.Errorf("reprogramar reserva %s: verificar estado: %w", id, err)
		}
		if recheckStatus == model.BookingStatusCancelled {
			return &SemanticError{
				Code:    ErrCodeInvalidInput,
				Message: "no se puede reprogramar una reserva cancelada",
			}
		}
		// status is pending or confirmed, so rowsAffected==0 means overlap
		return &SemanticError{
			Code:    ErrCodeBookingOverlap,
			Message: fmt.Sprintf("el Profesional %s ya tiene una reserva en ese horario.", professionalID),
		}
	}

	return nil
}

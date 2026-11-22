package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// AvailabilityBookingsReader is the ROLE-NEUTRAL bookings read adapter for the
// availability path (issue #243).
//
// Availability is a question about the calendar — "is this professional's window
// free?" — not about the caller's own records. Reading it through the
// caller-scoped BookingsRepo answered a different question: a client caller got
// `AND client_id = ?`, so every OTHER client's booking disappeared and the slot
// looked free; a staff caller got `AND professional_id = ?`, so asking about any
// other professional produced zero rows (or a contradictory clause) and the
// whole agenda looked free.
//
// It is a SEPARATE TYPE on purpose, not a flag, a ctx marker, a boolean
// parameter, or an `*Unscoped` method on the port (owner decision L4): the only
// way to get the unscoped answer is to have been wired to this adapter, so
// applyAuthFilter can never be bypassed by accident and no caller of the data
// tools can reach it by autocompletion.
//
// It deliberately does NOT call auth.RequireCaller (S6). Requiring a caller here
// would reintroduce the "not authenticated" failure into a path that by design
// does not require it — CheckAvailability is a non-authoritative preview tool,
// and find_available_slots answers its per-slot questions from an in-memory
// reader over the rows this adapter returns. Every DATA path keeps its caller
// requirement untouched (S5): this type implements two reads and nothing else.
//
// Both methods reuse the same builders the filtered BookingsRepo uses
// (bookingOverlapQuery, bookingRangeQuery). The ONLY difference from the
// filtered variants is the absent applyAuthFilter clause (S4).
type AvailabilityBookingsReader struct {
	db *sql.DB
}

// NewAvailabilityBookingsReader creates the availability read adapter over the
// given database handle.
func NewAvailabilityBookingsReader(db *sql.DB) *AvailabilityBookingsReader {
	return &AvailabilityBookingsReader{db: db}
}

// FindOverlapping returns every non-cancelled booking of staffID that overlaps
// the half-open [start, end) window, regardless of who owns it. It satisfies the
// narrow service.BookingOverlapReader the availability chain depends on.
func (r *AvailabilityBookingsReader) FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	query, args := bookingOverlapQuery(staffID, start, end)
	return queryBookings(ctx, r.db, "buscar reservas superpuestas (disponibilidad)", query, args)
}

// ListBookingsForRange returns every non-cancelled booking across all staff that
// overlaps the half-open [start, end) window, regardless of who owns it.
func (r *AvailabilityBookingsReader) ListBookingsForRange(ctx context.Context, start, end time.Time) ([]*entity.Booking, error) {
	query, args := bookingRangeQuery(start, end)
	return queryBookings(ctx, r.db, "listar reservas por rango (disponibilidad)", query, args)
}

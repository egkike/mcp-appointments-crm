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
// (bookingOverlapQuery, bookingRangeQuery): the PREDICATE — the WHERE and its
// args — is shared, so the unfiltered variant differs from the filtered one ONLY
// by the absent applyAuthFilter clause (S4 of fix-availability-scope). The
// PROJECTION is deliberately NOT shared (S3a, issue #247): these reads pass
// availabilityBookingColumns, the narrow list below.
//
// The projection is NARROW on purpose (L5, issue #247): the bookings returned
// here carry professional_id, start_datetime, end_datetime and status only,
// because that is all this path's consumers read (the validation chain's overlap
// step and the in-memory slot reader). ClientID, Notes, PaymentMethod,
// ServiceID, CreatedAt and UpdatedAt stay at their zero value because those
// columns are never selected.
//
// Trade-off (S6): containment here is STRUCTURAL, not procedural — the data is
// never loaded, so no echo of this adapter can leak it, and a future consumer
// that reads ClientID, Notes or PaymentMethod from these entities gets the zero
// value SILENTLY instead of a populated field. If a consumer ever needs one of
// them, widen the projection deliberately (and keep bookingColumns on the data
// paths, which must keep loading every column).
type AvailabilityBookingsReader struct {
	db *sql.DB
}

// NewAvailabilityBookingsReader creates the availability read adapter over the
// given database handle.
func NewAvailabilityBookingsReader(db *sql.DB) *AvailabilityBookingsReader {
	return &AvailabilityBookingsReader{db: db}
}

// scanAvailabilityBooking scans a row of the NARROW projection
// (availabilityBookingColumns) into an entity.Booking. It exists because
// scanBooking expects the full column list; the two scanners share
// scanBookingWindow, so the columns they have in common are mapped in exactly one
// place (issue #247 trap: neither scanner duplicates the other's mapping).
func scanAvailabilityBooking(scan func(dest ...any) error) (*entity.Booking, error) {
	var b entity.Booking
	var startStr, endStr string
	if err := scan(&b.ProfessionalID, &startStr, &endStr, (*string)(&b.Status)); err != nil {
		return nil, err
	}
	if err := scanBookingWindow(&b, startStr, endStr); err != nil {
		return nil, err
	}
	return &b, nil
}

// FindOverlapping returns every non-cancelled booking of staffID that overlaps
// the half-open [start, end) window, regardless of who owns it. It satisfies the
// narrow service.BookingOverlapReader the availability chain depends on.
func (r *AvailabilityBookingsReader) FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	query, args := bookingOverlapQuery(availabilityBookingColumns, staffID, start, end)
	return queryBookings(ctx, r.db, "buscar reservas superpuestas (disponibilidad)", query, args, scanAvailabilityBooking)
}

// ListBookingsForRange returns every non-cancelled booking across all staff that
// overlaps the half-open [start, end) window, regardless of who owns it.
func (r *AvailabilityBookingsReader) ListBookingsForRange(ctx context.Context, start, end time.Time) ([]*entity.Booking, error) {
	query, args := bookingRangeQuery(availabilityBookingColumns, start, end)
	return queryBookings(ctx, r.db, "listar reservas por rango (disponibilidad)", query, args, scanAvailabilityBooking)
}

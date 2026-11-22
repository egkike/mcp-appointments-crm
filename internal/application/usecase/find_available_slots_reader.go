package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// slotBookingsReader is the in-memory service.BookingOverlapReader injected into
// the validation chain. It answers every per-slot overlap question from the ONE
// booking read of the range (S9), so the shared chain keeps its overlap step
// without an N+1 query per candidate.
type slotBookingsReader struct {
	byProfessional map[string][]*entity.Booking
}

// newSlotBookingsReader indexes the range's bookings by professional, keeping
// only the non-cancelled ones (cancelled bookings never block a slot). Rows are
// kept in start order so the chain's "first overlapping booking" matches the
// repository's ORDER BY start_datetime ASC.
func newSlotBookingsReader(bookings []*entity.Booking) *slotBookingsReader {
	byProfessional := make(map[string][]*entity.Booking, len(bookings))
	for _, booking := range bookings {
		if booking == nil || booking.Status == entity.BookingStatusCancelled {
			continue
		}
		byProfessional[booking.ProfessionalID] = append(byProfessional[booking.ProfessionalID], booking)
	}
	for _, rows := range byProfessional {
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].StartDatetime.Before(rows[j].StartDatetime)
		})
	}
	return &slotBookingsReader{byProfessional: byProfessional}
}

// FindOverlapping reproduces BookingsRepo.FindOverlapping's semantics exactly:
// same professional, half-open interval comparison (start < end AND end > start)
// over non-cancelled bookings. Any divergence here would change which slots are
// filtered without a visible error, so the predicate is deliberately literal.
func (r *slotBookingsReader) FindOverlapping(_ context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error) {
	rows := r.byProfessional[staffID]
	overlapping := make([]*entity.Booking, 0, len(rows))
	for _, booking := range rows {
		if booking.StartDatetime.Before(end) && booking.EndDatetime.After(start) {
			overlapping = append(overlapping, booking)
		}
	}
	return overlapping, nil
}

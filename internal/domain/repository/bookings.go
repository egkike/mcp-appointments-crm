// Package repository defines the persistence contracts for the domain layer.
// Implementations live in the infrastructure layer; this package depends only
// on domain/entity and the Go standard library.
package repository

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// BookingsRepo defines the persistence contract for Booking aggregates.
// Implementations must return domain.ErrNotFound when a lookup by ID misses.
type BookingsRepo interface {
	// FindByID returns the booking with the given ID, or domain.ErrNotFound.
	FindByID(ctx context.Context, id string) (*entity.Booking, error)

	// Create persists a new booking. Returns domain.ErrConflict on overlap.
	Create(ctx context.Context, b *entity.Booking) error

	// Update replaces the booking record. The booking must already exist.
	Update(ctx context.Context, b *entity.Booking) error

	// Cancel transitions the booking to cancelled status by ID.
	Cancel(ctx context.Context, id string) error

	// Reschedule updates the start and end times of an existing booking.
	Reschedule(ctx context.Context, id string, newStart, newEnd time.Time) error

	// FindOverlapping returns bookings for the given staff member that overlap
	// the [start, end) window.
	FindOverlapping(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error)

	// FindByStaffAndRange returns all bookings for a staff member within a time range.
	FindByStaffAndRange(ctx context.Context, staffID string, start, end time.Time) ([]*entity.Booking, error)

	// ListBookingsForRange returns all bookings across all staff within a time range.
	ListBookingsForRange(ctx context.Context, start, end time.Time) ([]*entity.Booking, error)

	// SearchByNotes returns bookings whose notes match the query string.
	SearchByNotes(ctx context.Context, q string) ([]*entity.Booking, error)

	// UpdateStatus changes the status of a booking by ID.
	UpdateStatus(ctx context.Context, id string, status entity.BookingStatus) error
}

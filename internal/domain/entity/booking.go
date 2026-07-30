// Package entity contains the domain entities with business behavior.
// Entities are pure domain types with no infrastructure dependencies.
package entity

import "time"

// BookingStatus represents the FSM state of a booking.
type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
)

// Booking represents a service reservation.
// Datetime fields use time.Time for type safety (converted from string at the
// repository SQL-scan boundary).
type Booking struct {
	ID             string
	ClientID       string
	ProfessionalID string
	ServiceID      string
	StartDatetime  time.Time
	EndDatetime    time.Time
	Status         BookingStatus
	Notes          *string
	PaymentMethod  *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CanCancel reports whether the booking can be cancelled based on its status.
// Only pending and confirmed bookings can be cancelled.
func (b *Booking) CanCancel() bool {
	return b.Status == BookingStatusPending || b.Status == BookingStatusConfirmed
}

// CanReschedule reports whether the booking can be rescheduled based on its status.
// Only pending and confirmed bookings can be rescheduled.
func (b *Booking) CanReschedule() bool {
	return b.Status == BookingStatusPending || b.Status == BookingStatusConfirmed
}

// IsOverlapping reports whether this booking overlaps with another booking
// for the same professional. Two bookings overlap when one starts before the
// other ends AND ends after the other starts (strict inequality — adjacent
// bookings are not overlapping).
func (b *Booking) IsOverlapping(other *Booking) bool {
	return b.ProfessionalID == other.ProfessionalID &&
		b.StartDatetime.Before(other.EndDatetime) &&
		b.EndDatetime.After(other.StartDatetime)
}

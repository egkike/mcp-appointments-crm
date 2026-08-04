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

// ValidTransitions returns the set of statuses reachable from the current state.
// Implements the Booking FSM: pending → confirmed | cancelled, confirmed → cancelled,
// cancelled → terminal.
func (s BookingStatus) ValidTransitions() []BookingStatus {
	switch s {
	case BookingStatusPending:
		return []BookingStatus{BookingStatusConfirmed, BookingStatusCancelled}
	case BookingStatusConfirmed:
		return []BookingStatus{BookingStatusCancelled}
	case BookingStatusCancelled:
		return nil
	default:
		return nil
	}
}

// IsValidTransition reports whether transitioning from s to target is allowed
// by the Booking FSM.
func (s BookingStatus) IsValidTransition(target BookingStatus) bool {
	for _, v := range s.ValidTransitions() {
		if v == target {
			return true
		}
	}
	return false
}

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

// CanTransitionTo reports whether the booking can transition from its current
// status to the target status, following the Booking FSM rules.
func (b *Booking) CanTransitionTo(target BookingStatus) bool {
	return b.Status.IsValidTransition(target)
}

// ValidDuration reports whether the booking's duration (EndDatetime − StartDatetime)
// is positive and exactly matches the expected service duration.
// Zero-duration and negative-duration bookings are invalid.
func (b *Booking) ValidDuration(serviceDuration time.Duration) bool {
	dur := b.EndDatetime.Sub(b.StartDatetime)
	return dur > 0 && dur == serviceDuration
}

// IsValidTimeRange reports whether the booking's StartDatetime and EndDatetime
// form a valid time range: both must be non-zero and start must be strictly
// before end.
func (b *Booking) IsValidTimeRange() bool {
	if b.StartDatetime.IsZero() || b.EndDatetime.IsZero() {
		return false
	}
	return b.StartDatetime.Before(b.EndDatetime)
}

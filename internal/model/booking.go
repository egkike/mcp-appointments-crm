package model

// BookingStatus represents the FSM state of a booking.
type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
)

// ValidTransitions returns the set of statuses reachable from the current state.
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

// IsValidTransition reports whether transitioning from s to target is allowed.
func (s BookingStatus) IsValidTransition(target BookingStatus) bool {
	for _, v := range s.ValidTransitions() {
		if v == target {
			return true
		}
	}
	return false
}

// Booking represents a service reservation.
// ID is generated as UUID v4 by BookingsRepo.CreateBooking.
// EndDatetime is computed at insert time as start + service.duration_minutes.
type Booking struct {
	ID             string        `json:"id"`
	ClientID       string        `json:"client_id"`
	ProfessionalID string        `json:"professional_id"`
	ServiceID      string        `json:"service_id"`
	StartDatetime  string        `json:"start_datetime"`
	EndDatetime    string        `json:"end_datetime"`
	Status         BookingStatus `json:"status"`
	Notes          *string       `json:"notes"`
	PaymentMethod  *string       `json:"payment_method"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
}

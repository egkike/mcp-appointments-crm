package dto

import (
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// GetBookingInput holds the parameters for retrieving a single booking.
// The Caller field carries the authenticated actor for authorization;
// it is not serialized to JSON.
type GetBookingInput struct {
	// Caller is the authenticated actor requesting the booking.
	Caller auth.Caller `json:"-"`
	// BookingID identifies the booking to retrieve.
	BookingID string `json:"booking_id"`
}

// BookingView is a transport-ready representation of a booking.
// It decouples the API response shape from the domain entity, using
// time.Time for datetimes and plain strings for status.
type BookingView struct {
	// ID is the unique identifier of the booking.
	ID string `json:"id"`
	// ClientID identifies the customer.
	ClientID string `json:"client_id"`
	// ProfessionalID identifies the staff member providing the service.
	ProfessionalID string `json:"professional_id"`
	// ServiceID identifies the booked service.
	ServiceID string `json:"service_id"`
	// StartDatetime is the scheduled start of the booking.
	StartDatetime time.Time `json:"start_datetime"`
	// EndDatetime is the scheduled end of the booking.
	EndDatetime time.Time `json:"end_datetime"`
	// Status is the current booking status (e.g. "pending", "confirmed", "cancelled").
	Status string `json:"status"`
	// Notes is an optional free-text annotation.
	Notes *string `json:"notes,omitempty"`
	// PaymentMethod is an optional payment descriptor.
	PaymentMethod *string `json:"payment_method,omitempty"`
	// CreatedAt is the timestamp when the booking was first created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the timestamp of the last modification.
	UpdatedAt time.Time `json:"updated_at"`
}

// GetBookingResult holds the outcome of a successful get-booking request.
type GetBookingResult struct {
	// Booking is the transport-ready representation of the requested booking.
	Booking BookingView `json:"booking"`
}

package dto

import "github.com/egkike/mcp-appointments-crm/internal/auth"

// CancelBookingInput holds the parameters for cancelling an existing booking.
// The Caller field carries the authenticated actor for authorization;
// it is not serialized to JSON. Only Caller and BookingID are required.
type CancelBookingInput struct {
	// Caller is the authenticated actor requesting the cancellation.
	Caller auth.Caller `json:"-"`
	// BookingID identifies the booking to cancel.
	BookingID string `json:"booking_id"`
}

// CancelBookingResult holds the outcome of a successful cancellation.
type CancelBookingResult struct {
	// BookingID is the identifier of the cancelled booking.
	BookingID string `json:"booking_id"`
	// Status is the final booking status after cancellation (typically "cancelled").
	Status string `json:"status"`
}

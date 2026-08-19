package dto

import (
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// RescheduleBookingInput holds the parameters for rescheduling an existing booking.
// The Caller field carries the authenticated actor for authorization;
// it is not serialized to JSON.
type RescheduleBookingInput struct {
	// Caller is the authenticated actor requesting the reschedule.
	Caller auth.Caller `json:"-"`
	// BookingID identifies the booking to reschedule.
	BookingID string `json:"booking_id"`
	// NewStartTime is the requested new start time for the booking.
	NewStartTime time.Time `json:"new_start_time"`
}

// RescheduleBookingResult holds the outcome of a successful reschedule.
type RescheduleBookingResult struct {
	// BookingID is the identifier of the rescheduled booking.
	BookingID string `json:"booking_id"`
	// Status is the booking status after rescheduling (typically "pending" or "confirmed").
	Status string `json:"status"`
	// StartDatetime and EndDatetime are the new booking window
	// (REQ-MT-015 output contract). Populated by the use case — the MCP
	// transport has no repository access and cannot recompute the end time.
	StartDatetime time.Time `json:"start_datetime"`
	EndDatetime   time.Time `json:"end_datetime"`
}

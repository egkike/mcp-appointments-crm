// Package dto contains the application-layer data transfer objects.
// DTOs are pure data carriers with no business logic. They define the
// input/output contract between the transport layer and the use cases.
package dto

import (
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// CreateBookingInput holds the parameters for creating a new booking.
// The Caller field carries the authenticated actor for authorization;
// it is not serialized to JSON.
type CreateBookingInput struct {
	// Caller is the authenticated actor requesting the operation.
	Caller auth.Caller `json:"-"`
	// ClientID identifies the customer for whom the booking is created.
	ClientID string `json:"client_id"`
	// ServiceID identifies the service to book.
	ServiceID string `json:"service_id"`
	// ProfessionalID identifies the staff member who will provide the service.
	ProfessionalID string `json:"professional_id"`
	// StartTime is the requested start of the booking (UTC or wall-clock; the
	// use case converts to the business timezone as needed).
	StartTime time.Time `json:"start_time"`
	// Notes is an optional free-text annotation for the booking.
	Notes *string `json:"notes,omitempty"`
	// PaymentMethod is an optional descriptor of how the service will be paid.
	PaymentMethod *string `json:"payment_method,omitempty"`
}

// CreateBookingResult holds the outcome of a successful booking creation.
type CreateBookingResult struct {
	// BookingID is the unique identifier of the newly created booking.
	BookingID string `json:"booking_id"`
	// StartDatetime and EndDatetime are the computed booking window
	// (REQ-MT-015 output contract). Populated by the use case — the MCP
	// transport has no repository access and cannot recompute the end time.
	StartDatetime time.Time `json:"start_datetime"`
	EndDatetime   time.Time `json:"end_datetime"`
}

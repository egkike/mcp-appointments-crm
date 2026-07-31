package dto

import (
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// CheckAvailabilityParams holds the parameters for querying whether a specific
// datetime is available for booking. The Caller field carries the authenticated
// actor for authorization; it is not serialized to JSON.
type CheckAvailabilityParams struct {
	// Caller is the authenticated actor requesting the availability check.
	Caller auth.Caller `json:"-"`
	// ServiceID identifies the service being queried.
	ServiceID string `json:"service_id"`
	// ProfessionalID identifies the staff member whose calendar is queried.
	ProfessionalID string `json:"professional_id"`
	// StartDatetime is the desired start time in RFC3339 format. The use case
	// converts it to an RFC3339 string at the repository boundary.
	StartDatetime time.Time `json:"start_datetime"`
}

// CheckAvailabilityResult holds the outcome of an availability check.
type CheckAvailabilityResult struct {
	// Available is true when the requested datetime passes the full 5-step
	// validation chain (business hours, professional schedule, slot within
	// hours, no overlap, not in the past).
	Available bool `json:"available"`
}

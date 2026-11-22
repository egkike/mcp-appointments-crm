package dto

import (
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// FindAvailableSlotsInput holds the parameters of the date-range slot search
// (find_available_slots, agent-surface T2 / issue #241).
//
// The Caller field carries the authenticated actor for authorization; the tool
// is "any authenticated" (no RBAC entry in the composition root), so the use
// case re-asserts auth.RequireAuthenticated. Caller is never serialized.
//
// Dates travel as plain YYYY-MM-DD strings, not time.Time, because the range is
// a calendar window in the BUSINESS timezone: an offset-bearing instant would
// make the day boundaries depend on the caller's zone. The use case parses them
// and builds every candidate with time.Date(..., loc) in the business location.
type FindAvailableSlotsInput struct {
	// Caller is the authenticated actor requesting the search.
	Caller auth.Caller `json:"-"`
	// ServiceID identifies the service whose duration drives the slot grid.
	ServiceID string
	// ProfessionalID is optional. nil (or an empty/whitespace-only value) asks
	// for every ACTIVE professional that offers the service; a named value is
	// resolved strictly (unknown or inactive is a semantic error, never an
	// empty list).
	ProfessionalID *string
	// From is the first day of the range, inclusive, as YYYY-MM-DD.
	From string
	// To is the last day of the range, inclusive, as YYYY-MM-DD.
	To string
	// Limit caps the returned slots. nil means the default (10); the use case
	// rejects values outside 1..50 rather than silently clamping them.
	Limit *int
}

// AvailableSlot is one free slot of the range search.
//
// Response carrier: the JSON tags below ARE the wire contract (REQ-AV-3,
// REQ-MT-015) and are PII-free by construction — start + professional identity
// only. A slot filtered out by an existing client booking MUST NOT reveal that
// booking's client, and this carrier has no field able to.
type AvailableSlot struct {
	// StartDatetime is the slot start in the business location, so the value is
	// an unambiguous RFC3339 instant that can be fed back into create_booking.
	StartDatetime time.Time `json:"start_datetime"`
	// ProfessionalID identifies the staff member who is free at that time.
	ProfessionalID string `json:"professional_id"`
	// ProfessionalName is the display name of that staff member.
	ProfessionalName string `json:"professional_name"`
}

// FindAvailableSlotsResult is the response of the date-range slot search.
//
// Ordering and counting contract (REQ-AV-3): Slots is ordered by StartDatetime
// ascending and then by ProfessionalName; TotalCandidates counts every valid
// slot BEFORE Limit is applied; Truncated is true if and only if
// TotalCandidates > the applied limit. An empty range yields a non-nil empty
// Slots slice ([] on the wire, never null), TotalCandidates 0 and Truncated
// false — never an error.
type FindAvailableSlotsResult struct {
	// Slots are the free slots, capped by the applied limit.
	Slots []AvailableSlot `json:"slots"`
	// TotalCandidates is the number of valid slots before the limit.
	TotalCandidates int `json:"total_candidates"`
	// Truncated reports whether more valid slots existed than the limit allowed.
	Truncated bool `json:"truncated"`
}

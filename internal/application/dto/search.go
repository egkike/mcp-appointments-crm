package dto

import "github.com/egkike/mcp-appointments-crm/internal/auth"

// ClientSearchEntry is the pinned output contract for search_clients_advanced.
// Mirrors the entity.Client fields exposed by REQ-MT-015.
type ClientSearchEntry struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Phone       string  `json:"phone"`
	Preferences *string `json:"preferences,omitempty"`
}

// ServiceSearchEntry is the pinned output contract for search_services_advanced.
// Mirrors the entity.Service fields exposed by REQ-MT-015.
type ServiceSearchEntry struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	DurationMinutes int     `json:"duration_minutes"`
	Price           float64 `json:"price"`
	IsActive        bool    `json:"is_active"`
}

// ProfessionalSearchEntry is the pinned output contract for
// search_professionals_advanced (feat-owner-chat S7a). It is deliberately
// PII-free: the tool is readable by ANY authenticated caller (clients
// included), so the staff contact fields (email/phone) never cross into this
// carrier — email/phone stay reachable only through the owner-only write shape
// (professionalOut in the transport layer).
type ProfessionalSearchEntry struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RoleSpecialty *string  `json:"role_specialty,omitempty"`
	Specialties   []string `json:"specialties"`
}

// SearchClientsAdvancedInput carries the authenticated caller and the FTS query.
type SearchClientsAdvancedInput struct {
	Caller    auth.Caller `json:"-"`
	QueryText string
}

// SearchClientsAdvancedResult is the response body for search_clients_advanced.
type SearchClientsAdvancedResult struct {
	Results []ClientSearchEntry `json:"results"`
}

// SearchServicesAdvancedInput carries the authenticated caller and the FTS query.
type SearchServicesAdvancedInput struct {
	Caller    auth.Caller `json:"-"`
	QueryText string
}

// SearchServicesAdvancedResult is the response body for search_services_advanced.
type SearchServicesAdvancedResult struct {
	Results []ServiceSearchEntry `json:"results"`
}

// SearchProfessionalsAdvancedInput carries the authenticated caller, the
// optional text query and the optional service filter. Without both filters the
// use case returns the whole active catalog.
type SearchProfessionalsAdvancedInput struct {
	Caller    auth.Caller `json:"-"`
	QueryText string
	ServiceID string
}

// SearchProfessionalsAdvancedResult is the response body for
// search_professionals_advanced. Results is never nil on success, so the wire
// always renders an array (fail-closed: no relations loaded means [], never the
// full catalog).
type SearchProfessionalsAdvancedResult struct {
	Results []ProfessionalSearchEntry `json:"results"`
}

// LoyaltyReportEntry is a single row in the get_loyalty_report output.
// It exposes client PII (phone); access is restricted to owner/admin.
type LoyaltyReportEntry struct {
	ClientID     string `json:"client_id"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	BookingCount int    `json:"booking_count"`
}

// GetLoyaltyReportInput carries the authenticated caller, the report period and
// the optional top_n cap. TopN is a pointer so an explicit 0 can be clamped to 1
// while an omitted value defaults to 10.
type GetLoyaltyReportInput struct {
	Caller auth.Caller `json:"-"`
	Period string
	TopN   *int
}

// GetLoyaltyReportResult is the response body for get_loyalty_report.
type GetLoyaltyReportResult struct {
	Results []LoyaltyReportEntry `json:"results"`
}

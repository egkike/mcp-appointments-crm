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

// SearchClientsAdvancedInput carries the authenticated caller and the FTS query.
type SearchClientsAdvancedInput struct {
	Caller    auth.Caller `json:"-"`
	QueryText string      `json:"query_text"`
}

// SearchClientsAdvancedResult is the response body for search_clients_advanced.
type SearchClientsAdvancedResult struct {
	Results []ClientSearchEntry `json:"results"`
}

// SearchServicesAdvancedInput carries the authenticated caller and the FTS query.
type SearchServicesAdvancedInput struct {
	Caller    auth.Caller `json:"-"`
	QueryText string      `json:"query_text"`
}

// SearchServicesAdvancedResult is the response body for search_services_advanced.
type SearchServicesAdvancedResult struct {
	Results []ServiceSearchEntry `json:"results"`
}

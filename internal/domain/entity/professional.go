package entity

import "encoding/json"

// Professional represents a staff member who provides services.
type Professional struct {
	ID            string
	Name          string
	RoleSpecialty *string
	Status        string
	Email         *string
	Phone         *string
	Specialties   *string // JSON-encoded array of service IDs (e.g., `["svc-1","svc-2"]`), or nil
	CreatedAt     string
	UpdatedAt     string
}

// IsActive reports whether the professional is in active status.
func (p *Professional) IsActive() bool {
	return p.Status == "active"
}

// HasSpecialty reports whether the professional offers the given service.
// Specialties is a JSON-encoded array of service IDs (e.g., `["svc-1","svc-2"]`).
// Returns false if Specialties is nil, empty, or invalid JSON.
func (p *Professional) HasSpecialty(serviceID string) bool {
	if p.Specialties == nil || *p.Specialties == "" {
		return false
	}
	var serviceIDs []string
	if err := json.Unmarshal([]byte(*p.Specialties), &serviceIDs); err != nil {
		return false
	}
	for _, id := range serviceIDs {
		if id == serviceID {
			return true
		}
	}
	return false
}

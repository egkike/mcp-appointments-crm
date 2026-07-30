package entity

import "strings"

// Professional represents a staff member who provides services.
type Professional struct {
	ID            string
	Name          string
	RoleSpecialty *string
	Status        string
	Email         *string
	Phone         *string
	Specialties   *string // comma-separated list of service IDs
	CreatedAt     string
	UpdatedAt     string
}

// IsActive reports whether the professional is in active status.
func (p *Professional) IsActive() bool {
	return p.Status == "active"
}

// HasSpecialty reports whether the professional offers the given service.
// Specialties are stored as a comma-separated string; each entry is trimmed
// before comparison.
func (p *Professional) HasSpecialty(serviceID string) bool {
	if p.Specialties == nil || *p.Specialties == "" {
		return false
	}
	for _, s := range strings.Split(*p.Specialties, ",") {
		if strings.TrimSpace(s) == serviceID {
			return true
		}
	}
	return false
}

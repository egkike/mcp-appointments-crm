package entity

import "time"

// Service represents a bookable service in the business catalog.
type Service struct {
	ID              string
	Name            string
	Description     *string
	DurationMinutes int
	Price           float64
	Active          bool
	CreatedAt       string
	UpdatedAt       string
}

// IsActive reports whether the service is available for booking.
func (s *Service) IsActive() bool {
	return s.Active
}

// Duration returns the service duration as a time.Duration.
// Returns zero for non-positive DurationMinutes.
func (s *Service) Duration() time.Duration {
	if s.DurationMinutes <= 0 {
		return 0
	}
	return time.Duration(s.DurationMinutes) * time.Minute
}

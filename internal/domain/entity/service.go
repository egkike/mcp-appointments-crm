package entity

import (
	"fmt"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

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

// Validate checks business-rule invariants for a service.
// Name must be non-empty after trimming, DurationMinutes must be > 0,
// and Price must be > 0.
func (s *Service) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("el nombre no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	if s.DurationMinutes <= 0 {
		return fmt.Errorf("la duración debe ser mayor a 0 minutos: %w", domain.ErrInvalidInput)
	}
	if s.Price <= 0 {
		return fmt.Errorf("el precio debe ser mayor a 0: %w", domain.ErrInvalidInput)
	}
	return nil
}

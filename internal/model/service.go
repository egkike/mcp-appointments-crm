package model

// Service represents a bookable service in the business catalog.
type Service struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	DurationMinutes int     `json:"duration_minutes"`
	Price           float64 `json:"price"`
	IsActive        bool    `json:"is_active"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

package dto

import "github.com/egkike/mcp-appointments-crm/internal/auth"

// Maintenance DTOs (ADR-0015): the input/output contract for the eight
// operational-maintenance operations Hermes drives over business data
// (profile, services, professionals, weekly schedules).
//
// Every input carries the authenticated Caller so the use case can re-inject it
// into the context before the owner-only gate; it is never serialized to JSON.
// Partial-merge inputs are pointer-per-field: nil means "leave the stored value
// untouched", so the LLM can correct a single column without round-tripping the
// whole aggregate.

// UpdateBusinessProfileInput is the partial-merge payload for
// update_business_profile. Every updatable business_profile column is present
// and optional; identity and read-only columns (id, created_at, updated_at) are
// intentionally absent because they cannot be updated.
type UpdateBusinessProfileInput struct {
	Caller                 auth.Caller `json:"-"`
	Name                   *string     `json:"name,omitempty"`
	Industry               *string     `json:"industry,omitempty"`
	Country                *string     `json:"country,omitempty"`
	Address                *string     `json:"address,omitempty"`
	Latitude               *float64    `json:"latitude,omitempty"`
	Longitude              *float64    `json:"longitude,omitempty"`
	CoverPhotoURL          *string     `json:"cover_photo_url,omitempty"`
	PublicPhone            *string     `json:"public_phone,omitempty"`
	MessengerPlatform      *string     `json:"messenger_platform,omitempty"`
	MessengerID            *string     `json:"messenger_id,omitempty"`
	ContactEmail           *string     `json:"contact_email,omitempty"`
	WebsiteURL             *string     `json:"website_url,omitempty"`
	GeneralDescription     *string     `json:"general_description,omitempty"`
	CurrencyCode           *string     `json:"currency_code,omitempty"`
	CurrencySymbol         *string     `json:"currency_symbol,omitempty"`
	AcceptedPaymentMethods *string     `json:"accepted_payment_methods,omitempty"`
	Timezone               *string     `json:"timezone,omitempty"`
	SlotIntervalMinutes    *int        `json:"slot_interval_minutes,omitempty"`
	BusinessHours          *string     `json:"business_hours,omitempty"`
}

// CreateServiceInput is the full payload for create_service.
// Active is a pointer so an omitted is_active defaults to an active service
// (the overwhelmingly common intent) instead of the Go zero value (inactive).
type CreateServiceInput struct {
	Caller          auth.Caller `json:"-"`
	Name            string      `json:"name"`
	Description     *string     `json:"description,omitempty"`
	DurationMinutes int         `json:"duration_minutes"`
	Price           float64     `json:"price"`
	Active          *bool       `json:"is_active,omitempty"`
}

// UpdateServiceInput is the partial-merge payload for update_service.
// ServiceID identifies the service; every other field is optional (nil = keep
// the stored value).
type UpdateServiceInput struct {
	Caller          auth.Caller `json:"-"`
	ServiceID       string      `json:"service_id"`
	Name            *string     `json:"name,omitempty"`
	Description     *string     `json:"description,omitempty"`
	DurationMinutes *int        `json:"duration_minutes,omitempty"`
	Price           *float64    `json:"price,omitempty"`
	Active          *bool       `json:"is_active,omitempty"`
}

// DeleteServiceInput identifies the service to delete.
type DeleteServiceInput struct {
	Caller    auth.Caller `json:"-"`
	ServiceID string      `json:"service_id"`
}

// DeleteServiceResult is the response body for delete_service.
type DeleteServiceResult struct {
	ServiceID string `json:"service_id"`
	Status    string `json:"status"`
}

// CreateProfessionalInput is the full payload for create_professional.
// Specialties holds service IDs; the use case encodes them to the JSON array
// the professionals.specialties column stores. Status defaults to "active" when
// omitted.
type CreateProfessionalInput struct {
	Caller        auth.Caller `json:"-"`
	Name          string      `json:"name"`
	RoleSpecialty *string     `json:"role_specialty,omitempty"`
	Email         *string     `json:"email,omitempty"`
	Phone         *string     `json:"phone,omitempty"`
	Specialties   []string    `json:"specialties,omitempty"`
	Status        *string     `json:"status,omitempty"`
}

// UpdateProfessionalInput is the partial-merge payload for
// update_professional. Specialties is a pointer to a slice so an omitted list
// (nil) leaves the stored specialties untouched while a provided list (even an
// empty one) replaces them wholesale.
type UpdateProfessionalInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string      `json:"professional_id"`
	Name           *string     `json:"name,omitempty"`
	RoleSpecialty  *string     `json:"role_specialty,omitempty"`
	Email          *string     `json:"email,omitempty"`
	Phone          *string     `json:"phone,omitempty"`
	Specialties    *[]string   `json:"specialties,omitempty"`
	Status         *string     `json:"status,omitempty"`
}

// UpsertScheduleInput is the full day-slot payload for upsert_schedule.
// DayOfWeek follows Go's time.Weekday convention (0=Sunday..6=Saturday).
type UpsertScheduleInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string      `json:"professional_id"`
	DayOfWeek      int         `json:"day_of_week"`
	StartTime      string      `json:"start_time"`
	EndTime        string      `json:"end_time"`
}

// DeleteScheduleInput identifies the professional+day slot to remove.
type DeleteScheduleInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string      `json:"professional_id"`
	DayOfWeek      int         `json:"day_of_week"`
}

// DeleteScheduleResult is the response body for delete_schedule.
type DeleteScheduleResult struct {
	ProfessionalID string `json:"professional_id"`
	DayOfWeek      int    `json:"day_of_week"`
	Status         string `json:"status"`
}

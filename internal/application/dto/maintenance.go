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
//
// The partial-merge rule is "absent = keep the stored value; a non-nil pointer =
// overwrite" for every field, with one documented exception: the two location
// columns also accept an explicit clear. ClearLatitude/ClearLongitude carry
// that signal, because a *float64 alone cannot tell an absent latitude key from
// an explicit JSON null (both decode to nil). ClearLatitude/ClearLongitude are
// transport-to-application markers only and are never serialized.
type UpdateBusinessProfileInput struct {
	Caller                 auth.Caller `json:"-"`
	Name                   *string     `json:"name,omitempty"`
	Industry               *string     `json:"industry,omitempty"`
	Country                *string     `json:"country,omitempty"`
	Address                *string     `json:"address,omitempty"`
	Latitude               *float64    `json:"latitude,omitempty"`
	Longitude              *float64    `json:"longitude,omitempty"`
	ClearLatitude          bool        `json:"-"`
	ClearLongitude         bool        `json:"-"`
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

// CreateHoursExceptionInput is the full payload for create_hours_exception
// (feat-hours-exception-tool). The unique exception_date is the row identity;
// is_closed selects the only two legal shapes: a fully closed day (no times) or
// an open day with a custom window (both times). OpenTime/CloseTime/Reason are
// pointers because the entity stores them as pointers, so an omitted value
// stays NULL in SQLite — which is exactly what an is_closed=true row requires.
// Validation (date format/calendar, is_closed↔times, HH:MM, open<close,
// uniqueness) stays single-sourced in the repository (design §1.3); this carrier
// only bridges the transport to the use case.
type CreateHoursExceptionInput struct {
	Caller        auth.Caller `json:"-"`
	ExceptionDate string      `json:"exception_date"`
	IsClosed      bool        `json:"is_closed"`
	OpenTime      *string     `json:"open_time,omitempty"`
	CloseTime     *string     `json:"close_time,omitempty"`
	Reason        *string     `json:"reason,omitempty"`
}

// DeleteHoursExceptionInput identifies the exception to delete by its integer
// id, mirroring repository.BusinessHoursExceptionRepo.Delete(ctx, id). The id is
// the row identity the create read-back exposes to the LLM (design §2).
type DeleteHoursExceptionInput struct {
	Caller      auth.Caller `json:"-"`
	ExceptionID int         `json:"exception_id"`
}

// DeleteHoursExceptionResult is the response body for delete_hours_exception
// (mirrors dto.DeleteScheduleResult / dto.DeleteServiceResult).
type DeleteHoursExceptionResult struct {
	ExceptionID int    `json:"exception_id"`
	Status      string `json:"status"`
}

// ListHoursExceptionsInput is the required inclusive range for
// list_hours_exceptions. Both bounds are YYYY-MM-DD strings; they are parsed and
// format-checked at the use case before the repository call (a malformed bound
// is a Spanish semantic error). The range mirrors
// repository.BusinessHoursExceptionRepo.List: inclusive on both ends, with no
// invented default window (design §3, D4).
type ListHoursExceptionsInput struct {
	Caller auth.Caller `json:"-"`
	From   string      `json:"from"`
	To     string      `json:"to"`
}

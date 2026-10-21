package dto

import (
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

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
	Name                   *string
	Industry               *string
	Country                *string
	Address                *string
	Latitude               *float64
	Longitude              *float64
	ClearLatitude          bool `json:"-"`
	ClearLongitude         bool `json:"-"`
	CoverPhotoURL          *string
	PublicPhone            *string
	MessengerPlatform      *string
	MessengerID            *string
	ContactEmail           *string
	WebsiteURL             *string
	GeneralDescription     *string
	CurrencyCode           *string
	CurrencySymbol         *string
	AcceptedPaymentMethods *string
	Timezone               *string
	SlotIntervalMinutes    *int
	BusinessHours          *string
}

// CreateServiceInput is the full payload for create_service.
// Active is a pointer so an omitted is_active defaults to an active service
// (the overwhelmingly common intent) instead of the Go zero value (inactive).
type CreateServiceInput struct {
	Caller          auth.Caller `json:"-"`
	Name            string
	Description     *string
	DurationMinutes int
	Price           float64
	Active          *bool
}

// UpdateServiceInput is the partial-merge payload for update_service.
// ServiceID identifies the service; every other field is optional (nil = keep
// the stored value).
type UpdateServiceInput struct {
	Caller          auth.Caller `json:"-"`
	ServiceID       string
	Name            *string
	Description     *string
	DurationMinutes *int
	Price           *float64
	Active          *bool
}

// DeleteServiceInput identifies the service to delete.
type DeleteServiceInput struct {
	Caller    auth.Caller `json:"-"`
	ServiceID string
}

// DeleteServiceResult is the cross-layer result carrier of delete_service.
// Transport-neutral by contract (ADR-0013 boundary contract 4): the wire shape
// and its json tags live in internal/mcp (deleteServiceOut), which maps this
// carrier field by field. Do not add json tags here.
type DeleteServiceResult struct {
	ServiceID string
	Status    string
}

// CreateProfessionalInput is the full payload for create_professional.
// Specialties holds service IDs; the use case encodes them to the JSON array
// the professionals.specialties column stores. Status defaults to "active" when
// omitted.
type CreateProfessionalInput struct {
	Caller        auth.Caller `json:"-"`
	Name          string
	RoleSpecialty *string
	Email         *string
	Phone         *string
	Specialties   []string
	Status        *string
}

// UpdateProfessionalInput is the partial-merge payload for
// update_professional. Specialties is a pointer to a slice so an omitted list
// (nil) leaves the stored specialties untouched while a provided list (even an
// empty one) replaces them wholesale.
type UpdateProfessionalInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string
	Name           *string
	RoleSpecialty  *string
	Email          *string
	Phone          *string
	Specialties    *[]string
	Status         *string
}

// UpsertScheduleInput is the full day-slot payload for upsert_schedule.
// DayOfWeek follows Go's time.Weekday convention (0=Sunday..6=Saturday).
type UpsertScheduleInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string
	DayOfWeek      int
	StartTime      string
	EndTime        string
}

// DeleteScheduleInput identifies the professional+day slot to remove.
type DeleteScheduleInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string
	DayOfWeek      int
}

// DeleteScheduleResult is the cross-layer result carrier of delete_schedule.
// Transport-neutral by contract (ADR-0013 boundary contract 4): the wire shape
// and its json tags live in internal/mcp (deleteScheduleOut), which maps this
// carrier field by field. Do not add json tags here.
type DeleteScheduleResult struct {
	ProfessionalID string
	DayOfWeek      int
	Status         string
}

// CreateHoursExceptionInput is the full payload for create_hours_exception
// (feat-hours-exception-tool). Application-layer carrier only — no data-field
// json tags: the transport adapter owns the wire shape (its own
// createHoursExceptionIn struct) and maps field-by-field into this DTO. The
// unique exception_date is the row identity;
// is_closed selects the only two legal shapes: a fully closed day (no times) or
// an open day with a custom window (both times). OpenTime/CloseTime/Reason are
// pointers because the entity stores them as pointers, so an omitted value
// stays NULL in SQLite — which is exactly what an is_closed=true row requires.
// Validation (date format/calendar, is_closed↔times, HH:MM, open<close,
// uniqueness) stays single-sourced in the repository (design §1.3).
// Caller stays `json:"-"` protectively: a serialization accident here must
// never leak the caller identity into a wire payload.
type CreateHoursExceptionInput struct {
	Caller        auth.Caller `json:"-"`
	ExceptionDate string
	IsClosed      bool
	OpenTime      *string
	CloseTime     *string
	Reason        *string
}

// DeleteHoursExceptionInput identifies the exception to delete by its integer
// id, mirroring repository.BusinessHoursExceptionRepo.Delete(ctx, id). The id is
// the row identity the create read-back exposes to the LLM (design §2).
// Application-layer carrier only — the transport owns deleteHoursExceptionIn
// on the wire; Caller stays `json:"-"` protectively.
type DeleteHoursExceptionInput struct {
	Caller      auth.Caller `json:"-"`
	ExceptionID int
}

// DeleteHoursExceptionResult is the cross-layer result carrier of
// delete_hours_exception (mirrors dto.DeleteScheduleResult /
// dto.DeleteServiceResult). Transport-neutral by contract (ADR-0013 boundary
// contract 4): the wire shape and its json tags live in internal/mcp
// (deleteHoursExceptionOut), which maps this carrier field by field. Do not add
// json tags here.
type DeleteHoursExceptionResult struct {
	ExceptionID int
	Status      string
}

// GetProfessionalSchedulesInput identifies the professional whose weekly agenda
// is read (feat-owner-chat S7b). Application-layer carrier only — the transport
// owns getProfessionalSchedulesIn on the wire, so this struct carries no data
// json tags; Caller stays `json:"-"` protectively.
type GetProfessionalSchedulesInput struct {
	Caller         auth.Caller `json:"-"`
	ProfessionalID string
}

// ProfessionalScheduleSubject is the identity the agenda read returns next to
// the rows (design §8.3 D4): the conversational delete gate must be able to name
// whom it is about to confirm instead of repeating a raw id. Only id, name and
// status cross into this carrier — email/phone never do, even though the entity
// carries them.
type ProfessionalScheduleSubject struct {
	ID     string
	Name   string
	Status string
}

// GetProfessionalSchedulesResult is the response carrier of
// get_professional_schedules: the professional identity plus every stored weekly
// slot, already ordered by day_of_week by the repository port. Schedules is
// never nil on success so the transport always renders an array (a professional
// without an agenda answers [] and not NOT_FOUND). Rows travel as entities
// because the transport reuses the pinned scheduleOut wire shape, keeping one
// schedule shape on the wire (design §8.3).
type GetProfessionalSchedulesResult struct {
	Professional ProfessionalScheduleSubject
	Schedules    []*entity.Schedule
}

// ListHoursExceptionsInput is the required inclusive range for
// list_hours_exceptions. Application-layer carrier only — the transport owns
// listHoursExceptionsIn on the wire; Caller stays `json:"-"` protectively.
// Both bounds are YYYY-MM-DD strings; they are parsed and
// format-checked at the use case before the repository call (a malformed bound
// is a Spanish semantic error). The range mirrors
// repository.BusinessHoursExceptionRepo.List: inclusive on both ends, with no
// invented default window (design §3, D4).
type ListHoursExceptionsInput struct {
	Caller auth.Caller `json:"-"`
	From   string
	To     string
}

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Maintenance WRITE tools (ADR-0015, T3). These eight tools expose the T2 use
// cases over the MCP surface so Hermes can correct post-install business data
// (profile, services, professionals, weekly schedules) without manual SQL.
// Every tool is owner-only: the ToolRBAC entry in the composition root
// (cmd/mcp-server/main.go) is the coarse transport gate and every use case
// re-asserts auth.RequireRole(RoleOwner) — both are required (defense in
// depth, ADR-0015 Decision 2).
//
// Validation split (ADR-0015 drift mitigation): a handler enforces ONLY
// transport schema shape. JSON types and required fields come from the struct
// tags via the SDK; on top of that the schedule tools check the day_of_week
// range and the HH:MM shape because a malformed time is cheap to reject at the
// edge and the LLM gets a better message than a repository error. Every
// business rule (non-empty name, positive duration and price, valid status,
// business_hours JSON, specialty existence, day range, open<close) stays in
// entity.Validate / the use case / the repository, so the MCP surface and the
// admin TUI cannot drift.

// maintenanceHHMM mirrors the zero-padded 24-hour HH:MM shape enforced by
// entity.BusinessProfile and repository.SchedulesRepo. The transport keeps its
// own copy so it never imports repository/domain policy (same reasoning as
// maxFTSQueryLen in tools_search.go).
var maintenanceHHMM = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Day-of-week bounds of the Go time.Weekday encoding exposed to Hermes
// (0=Sunday..6=Saturday), matching repository.SchedulesRepo.
const (
	dayOfWeekSunday   = 0
	dayOfWeekSaturday = 6
)

// errNilMaintenanceEntity is returned when a maintenance port reports success
// without the entity/result it was supposed to produce. The T2 use cases never
// do this; the guard fails closed to -32603 instead of dereferencing nil or
// emitting a half-empty JSON payload (same class of guard as the
// get_business_profile nil check, GGA S-1).
var errNilMaintenanceEntity = errors.New("maintenance tool: use case returned no result")

// updateBusinessProfileIn is the input of update_business_profile. It mirrors
// dto.UpdateBusinessProfileInput (the 19 writable business_profile columns):
// every field is a pointer, so an omitted field means "leave the stored value
// untouched" and the LLM can correct a single column without round-tripping the
// whole aggregate. id/created_at/updated_at are intentionally absent — they are
// not writable.
type updateBusinessProfileIn struct {
	Name                   *string  `json:"name,omitempty"`
	Industry               *string  `json:"industry,omitempty"`
	Country                *string  `json:"country,omitempty"`
	Address                *string  `json:"address,omitempty"`
	Latitude               *float64 `json:"latitude,omitempty"`
	Longitude              *float64 `json:"longitude,omitempty"`
	CoverPhotoURL          *string  `json:"cover_photo_url,omitempty"`
	PublicPhone            *string  `json:"public_phone,omitempty"`
	MessengerPlatform      *string  `json:"messenger_platform,omitempty"`
	MessengerID            *string  `json:"messenger_id,omitempty"`
	ContactEmail           *string  `json:"contact_email,omitempty"`
	WebsiteURL             *string  `json:"website_url,omitempty"`
	GeneralDescription     *string  `json:"general_description,omitempty"`
	CurrencyCode           *string  `json:"currency_code,omitempty"`
	CurrencySymbol         *string  `json:"currency_symbol,omitempty"`
	AcceptedPaymentMethods *string  `json:"accepted_payment_methods,omitempty"`
	Timezone               *string  `json:"timezone,omitempty"`
	SlotIntervalMinutes    *int     `json:"slot_interval_minutes,omitempty"`
	BusinessHours          *string  `json:"business_hours,omitempty"`
}

// toUpdateBusinessProfileInput maps the tool payload onto the use case input,
// injecting the authenticated caller.
func toUpdateBusinessProfileInput(in updateBusinessProfileIn, caller auth.Caller) dto.UpdateBusinessProfileInput {
	return dto.UpdateBusinessProfileInput{
		Caller:                 caller,
		Name:                   in.Name,
		Industry:               in.Industry,
		Country:                in.Country,
		Address:                in.Address,
		Latitude:               in.Latitude,
		Longitude:              in.Longitude,
		CoverPhotoURL:          in.CoverPhotoURL,
		PublicPhone:            in.PublicPhone,
		MessengerPlatform:      in.MessengerPlatform,
		MessengerID:            in.MessengerID,
		ContactEmail:           in.ContactEmail,
		WebsiteURL:             in.WebsiteURL,
		GeneralDescription:     in.GeneralDescription,
		CurrencyCode:           in.CurrencyCode,
		CurrencySymbol:         in.CurrencySymbol,
		AcceptedPaymentMethods: in.AcceptedPaymentMethods,
		Timezone:               in.Timezone,
		SlotIntervalMinutes:    in.SlotIntervalMinutes,
		BusinessHours:          in.BusinessHours,
	}
}

// createServiceIn is the input of create_service: the full service payload.
// Active is optional and defaults to an active service in the use case (an
// omitted flag must not hide the service).
type createServiceIn struct {
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	DurationMinutes int     `json:"duration_minutes"`
	Price           float64 `json:"price"`
	Active          *bool   `json:"is_active,omitempty"`
}

// updateServiceIn is the input of update_service: ServiceID identifies the
// service, every other field is optional (nil = keep the stored value).
type updateServiceIn struct {
	ServiceID       string   `json:"service_id"`
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	DurationMinutes *int     `json:"duration_minutes,omitempty"`
	Price           *float64 `json:"price,omitempty"`
	Active          *bool    `json:"is_active,omitempty"`
}

// deleteServiceIn is the input of delete_service.
type deleteServiceIn struct {
	ServiceID string `json:"service_id"`
}

// createProfessionalIn is the input of create_professional: the full staff
// payload. Specialties holds service IDs; Status is optional and defaults to
// "active" in the use case.
type createProfessionalIn struct {
	Name          string   `json:"name"`
	RoleSpecialty *string  `json:"role_specialty,omitempty"`
	Email         *string  `json:"email,omitempty"`
	Phone         *string  `json:"phone,omitempty"`
	Specialties   []string `json:"specialties,omitempty"`
	Status        *string  `json:"status,omitempty"`
}

// updateProfessionalIn is the input of update_professional: ProfessionalID
// identifies the row, every other field is optional. Specialties is a pointer
// to a slice so an omitted list keeps the stored specialties while a provided
// list (even an empty one) replaces them wholesale.
type updateProfessionalIn struct {
	ProfessionalID string    `json:"professional_id"`
	Name           *string   `json:"name,omitempty"`
	RoleSpecialty  *string   `json:"role_specialty,omitempty"`
	Email          *string   `json:"email,omitempty"`
	Phone          *string   `json:"phone,omitempty"`
	Specialties    *[]string `json:"specialties,omitempty"`
	Status         *string   `json:"status,omitempty"`
}

// upsertScheduleIn is the input of upsert_schedule. DayOfWeek follows Go's
// time.Weekday encoding (0=Sunday..6=Saturday), matching repository
// schedules.day_of_week; StartTime and EndTime are zero-padded 24-hour HH:MM.
type upsertScheduleIn struct {
	ProfessionalID string `json:"professional_id"`
	DayOfWeek      int    `json:"day_of_week"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
}

// deleteScheduleIn is the input of delete_schedule.
type deleteScheduleIn struct {
	ProfessionalID string `json:"professional_id"`
	DayOfWeek      int    `json:"day_of_week"`
}

// serviceOut is the pinned service write shape. It reuses the
// search_services_advanced entry (dto.ServiceSearchEntry) so Hermes sees one
// service object across the read and write tools, exactly like
// update_business_profile reuses businessProfileOut.
type serviceOut = dto.ServiceSearchEntry

// toServiceOut maps a stored service to the pinned service shape. created_at /
// updated_at stay out of the contract: the FTS read shape does not expose them
// and save/update return the row without re-reading them consistently.
func toServiceOut(s *entity.Service) serviceOut {
	return serviceOut{
		ID:              s.ID,
		Name:            s.Name,
		Description:     s.Description,
		DurationMinutes: s.DurationMinutes,
		Price:           s.Price,
		IsActive:        s.Active,
	}
}

// professionalOut is the pinned professional write shape. specialties is
// stored as a JSON array of service IDs and is exposed as a list again, so the
// output mirrors the input type.
type professionalOut struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RoleSpecialty *string  `json:"role_specialty,omitempty"`
	Status        string   `json:"status"`
	Email         *string  `json:"email,omitempty"`
	Phone         *string  `json:"phone,omitempty"`
	Specialties   []string `json:"specialties"`
}

// toProfessionalOut maps a stored professional to the pinned shape. Specialties
// fails safe to an empty list when the stored value is absent or unreadable,
// mirroring entity.Professional.HasSpecialty (invalid JSON means "no
// specialties"): the LLM always receives a list, never a raw JSON string.
func toProfessionalOut(p *entity.Professional) professionalOut {
	return professionalOut{
		ID:            p.ID,
		Name:          p.Name,
		RoleSpecialty: p.RoleSpecialty,
		Status:        p.Status,
		Email:         p.Email,
		Phone:         p.Phone,
		Specialties:   decodeSpecialties(p.Specialties),
	}
}

// decodeSpecialties decodes the JSON-encoded service IDs stored in
// professionals.specialties. nil, empty, JSON null and invalid values all map
// to an empty (non-nil) list so the JSON output is always an array.
func decodeSpecialties(stored *string) []string {
	ids := []string{}
	if stored == nil || *stored == "" {
		return ids
	}
	if err := json.Unmarshal([]byte(*stored), &ids); err != nil {
		return []string{}
	}
	if ids == nil {
		// "null" unmarshals into a nil slice; keep the array contract.
		return []string{}
	}
	return ids
}

// scheduleOut is the pinned weekly-slot write shape (the row as stored,
// including the SQLite-assigned id).
type scheduleOut struct {
	ID             int    `json:"id"`
	ProfessionalID string `json:"professional_id"`
	DayOfWeek      int    `json:"day_of_week"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
}

// toScheduleOut maps a stored schedule row to the pinned shape.
func toScheduleOut(s *entity.Schedule) scheduleOut {
	return scheduleOut{
		ID:             s.ID,
		ProfessionalID: s.ProfessionalID,
		DayOfWeek:      s.DayOfWeek,
		StartTime:      s.StartTime,
		EndTime:        s.EndTime,
	}
}

// registerMaintenanceTools wires the eight owner-only maintenance WRITE tools
// onto the SDK server, one sub-registrar per entity family. Each sub-registrar
// skips its tools when the matching port is nil, keeping the skeleton behavior
// (tool absent) that transport-level tests rely on.
func (s *Server) registerMaintenanceTools() {
	s.registerBusinessProfileWriteTools()
	s.registerServiceWriteTools()
	s.registerProfessionalWriteTools()
	s.registerScheduleWriteTools()
}

// registerBusinessProfileWriteTools wires update_business_profile. The output
// is the same businessProfileOut shape get_business_profile returns, so Hermes
// reads back exactly what it wrote.
func (s *Server) registerBusinessProfileWriteTools() {
	if s.cfg.UpdateBusinessProfile == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("update_business_profile", "Actualiza parcialmente el perfil del negocio: solo los campos enviados se modifican (business_hours se reemplaza completo, no se fusiona). Solo disponible para owner"),
		func(ctx context.Context, _ *mcp.CallToolRequest, in updateBusinessProfileIn) (*mcp.CallToolResult, businessProfileOut, error) {
			caller, err := auth.RequireCaller(ctx)
			if err != nil {
				return nil, businessProfileOut{}, toMCPError(err)
			}
			profile, err := s.cfg.UpdateBusinessProfile.Execute(ctx, toUpdateBusinessProfileInput(in, *caller))
			if err != nil {
				return nil, businessProfileOut{}, toMCPError(err)
			}
			if profile == nil {
				return nil, businessProfileOut{}, toMCPError(errNilMaintenanceEntity)
			}
			return nil, toBusinessProfileOut(profile), nil
		})
	s.toolNames["update_business_profile"] = struct{}{}
}

// registerServiceWriteTools wires the three catalog tools: create, partial
// update and delete. All three return the pinned serviceOut shape, except the
// delete confirmation, which is the minimal result DTO (mirroring
// mark_alert_as_sent).
func (s *Server) registerServiceWriteTools() {
	if s.cfg.CreateService != nil {
		mcp.AddTool(s.impl, s.mcpTool("create_service", "Crea un servicio del catálogo. is_active es opcional y por defecto el servicio se crea activo. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in createServiceIn) (*mcp.CallToolResult, serviceOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, serviceOut{}, toMCPError(err)
				}
				service, err := s.cfg.CreateService.Execute(ctx, dto.CreateServiceInput{
					Caller:          *caller,
					Name:            in.Name,
					Description:     in.Description,
					DurationMinutes: in.DurationMinutes,
					Price:           in.Price,
					Active:          in.Active,
				})
				if err != nil {
					return nil, serviceOut{}, toMCPError(err)
				}
				if service == nil {
					return nil, serviceOut{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, toServiceOut(service), nil
			})
		s.toolNames["create_service"] = struct{}{}
	}

	if s.cfg.UpdateService != nil {
		mcp.AddTool(s.impl, s.mcpTool("update_service", "Actualiza parcialmente un servicio existente: solo los campos enviados se modifican. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in updateServiceIn) (*mcp.CallToolResult, serviceOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, serviceOut{}, toMCPError(err)
				}
				service, err := s.cfg.UpdateService.Execute(ctx, dto.UpdateServiceInput{
					Caller:          *caller,
					ServiceID:       in.ServiceID,
					Name:            in.Name,
					Description:     in.Description,
					DurationMinutes: in.DurationMinutes,
					Price:           in.Price,
					Active:          in.Active,
				})
				if err != nil {
					return nil, serviceOut{}, toMCPError(err)
				}
				if service == nil {
					return nil, serviceOut{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, toServiceOut(service), nil
			})
		s.toolNames["update_service"] = struct{}{}
	}

	if s.cfg.DeleteService != nil {
		mcp.AddTool(s.impl, s.mcpTool("delete_service", "Elimina un servicio del catálogo. Falla si el servicio tiene reservas asociadas. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in deleteServiceIn) (*mcp.CallToolResult, dto.DeleteServiceResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.DeleteServiceResult{}, toMCPError(err)
				}
				result, err := s.cfg.DeleteService.Execute(ctx, dto.DeleteServiceInput{
					Caller:    *caller,
					ServiceID: in.ServiceID,
				})
				if err != nil {
					return nil, dto.DeleteServiceResult{}, toMCPError(err)
				}
				if result == nil {
					return nil, dto.DeleteServiceResult{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, *result, nil
			})
		s.toolNames["delete_service"] = struct{}{}
	}
}

// registerProfessionalWriteTools wires the two staff tools. There is no
// professional read tool yet, so professionalOut is the pinned shape of this
// surface.
func (s *Server) registerProfessionalWriteTools() {
	if s.cfg.CreateProfessional != nil {
		mcp.AddTool(s.impl, s.mcpTool("create_professional", "Crea un profesional. specialties recibe IDs de servicios existentes; status es opcional y por defecto es active. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in createProfessionalIn) (*mcp.CallToolResult, professionalOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, professionalOut{}, toMCPError(err)
				}
				professional, err := s.cfg.CreateProfessional.Execute(ctx, dto.CreateProfessionalInput{
					Caller:        *caller,
					Name:          in.Name,
					RoleSpecialty: in.RoleSpecialty,
					Email:         in.Email,
					Phone:         in.Phone,
					Specialties:   in.Specialties,
					Status:        in.Status,
				})
				if err != nil {
					return nil, professionalOut{}, toMCPError(err)
				}
				if professional == nil {
					return nil, professionalOut{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, toProfessionalOut(professional), nil
			})
		s.toolNames["create_professional"] = struct{}{}
	}

	if s.cfg.UpdateProfessional != nil {
		mcp.AddTool(s.impl, s.mcpTool("update_professional", "Actualiza parcialmente un profesional existente: solo los campos enviados se modifican (specialties reemplaza la lista completa). Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in updateProfessionalIn) (*mcp.CallToolResult, professionalOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, professionalOut{}, toMCPError(err)
				}
				professional, err := s.cfg.UpdateProfessional.Execute(ctx, dto.UpdateProfessionalInput{
					Caller:         *caller,
					ProfessionalID: in.ProfessionalID,
					Name:           in.Name,
					RoleSpecialty:  in.RoleSpecialty,
					Email:          in.Email,
					Phone:          in.Phone,
					Specialties:    in.Specialties,
					Status:         in.Status,
				})
				if err != nil {
					return nil, professionalOut{}, toMCPError(err)
				}
				if professional == nil {
					return nil, professionalOut{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, toProfessionalOut(professional), nil
			})
		s.toolNames["update_professional"] = struct{}{}
	}
}

// registerScheduleWriteTools wires the two weekly-agenda tools. Both check the
// day_of_week range at the transport; upsert additionally checks the HH:MM
// shape of both times.
func (s *Server) registerScheduleWriteTools() {
	if s.cfg.UpsertSchedule != nil {
		mcp.AddTool(s.impl, s.mcpTool("upsert_schedule", "Crea o reemplaza el horario de un profesional para un día de la semana. day_of_week usa el encoding de Go: 0=domingo, 1=lunes, ..., 6=sábado; start_time y end_time son HH:MM en 24h (ej. 09:30). Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in upsertScheduleIn) (*mcp.CallToolResult, scheduleOut, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, scheduleOut{}, toMCPError(err)
				}
				if err := validateDayOfWeek(in.DayOfWeek); err != nil {
					return nil, scheduleOut{}, toMCPError(err)
				}
				if !maintenanceHHMM.MatchString(in.StartTime) {
					return nil, scheduleOut{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: "start_time debe tener formato HH:MM en 24h (ej. 09:30)",
					})
				}
				if !maintenanceHHMM.MatchString(in.EndTime) {
					return nil, scheduleOut{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: "end_time debe tener formato HH:MM en 24h (ej. 18:00)",
					})
				}
				schedule, err := s.cfg.UpsertSchedule.Execute(ctx, dto.UpsertScheduleInput{
					Caller:         *caller,
					ProfessionalID: in.ProfessionalID,
					DayOfWeek:      in.DayOfWeek,
					StartTime:      in.StartTime,
					EndTime:        in.EndTime,
				})
				if err != nil {
					return nil, scheduleOut{}, toMCPError(err)
				}
				if schedule == nil {
					return nil, scheduleOut{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, toScheduleOut(schedule), nil
			})
		s.toolNames["upsert_schedule"] = struct{}{}
	}

	if s.cfg.DeleteSchedule != nil {
		mcp.AddTool(s.impl, s.mcpTool("delete_schedule", "Elimina el horario de un profesional para un día de la semana. day_of_week usa el encoding de Go: 0=domingo, 1=lunes, ..., 6=sábado. Solo disponible para owner"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in deleteScheduleIn) (*mcp.CallToolResult, dto.DeleteScheduleResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.DeleteScheduleResult{}, toMCPError(err)
				}
				if err := validateDayOfWeek(in.DayOfWeek); err != nil {
					return nil, dto.DeleteScheduleResult{}, toMCPError(err)
				}
				result, err := s.cfg.DeleteSchedule.Execute(ctx, dto.DeleteScheduleInput{
					Caller:         *caller,
					ProfessionalID: in.ProfessionalID,
					DayOfWeek:      in.DayOfWeek,
				})
				if err != nil {
					return nil, dto.DeleteScheduleResult{}, toMCPError(err)
				}
				if result == nil {
					return nil, dto.DeleteScheduleResult{}, toMCPError(errNilMaintenanceEntity)
				}
				return nil, *result, nil
			})
		s.toolNames["delete_schedule"] = struct{}{}
	}
}

// validateDayOfWeek is the shared transport check for both schedule tools. The
// message mirrors the repository message so Hermes sees one wording for the
// same rule.
func validateDayOfWeek(day int) error {
	if day < dayOfWeekSunday || day > dayOfWeekSaturday {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el día debe estar entre 0 (domingo) y 6 (sábado)",
		}
	}
	return nil
}

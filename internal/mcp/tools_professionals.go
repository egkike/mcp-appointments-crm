package mcp

import (
	"context"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// searchProfessionalsAdvancedIn is the input of search_professionals_advanced.
// Both fields are optional: without them the tool returns the whole active
// catalog. query_text is a plain substring (there is no professionals_fts table);
// the shared FTS query validator still rejects operator syntax so all the
// *_advanced tools share one text policy.
type searchProfessionalsAdvancedIn struct {
	QueryText string `json:"query_text,omitempty"`
	ServiceID string `json:"service_id,omitempty"`
}

// registerProfessionalsReadTools wires the professionals read tools onto the SDK
// server when the corresponding port is non-nil.
//
// search_professionals_advanced admits ANY authenticated caller (feat-owner-chat
// S7a): it carries no ToolRBAC entry, the transport only resolves the caller, and
// the use case enforces auth.RequireAuthenticated. This is deliberately NOT the
// search_services_advanced shape (owner/admin in the use case): the conversational
// booking flow belongs to the client, and the output shape is PII-free
// (dto.ProfessionalSearchEntry has no email/phone), so exposing it to a client is
// safe by construction.
//
// get_professional (feat-staff-self-read S1, issue #276) is the single-record
// read: owner, admin and staff, with the client excluded, and email/phone crossing
// only for the owner and the professional itself (S4).
//
// get_professional_schedules is the agenda read (S7b), widened to owner, admin
// and staff by S3 of the same change; staff is own-only at the use-case layer.
// Both tools carry a ToolRBAC row in the composition root AND re-assert the role
// inside the use case, so the transport keeps only resolving the caller and
// mapping errors.
func (s *Server) registerProfessionalsReadTools() {
	s.registerSearchProfessionalsAdvancedTool()
	s.registerGetProfessionalTool()
	s.registerGetProfessionalSchedulesTool()
}

// registerSearchProfessionalsAdvancedTool wires the PII-free catalog read.
func (s *Server) registerSearchProfessionalsAdvancedTool() {
	if s.cfg.SearchProfessionalsAdvanced == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("search_professionals_advanced",
		"Busca profesionales activos por nombre o especialidad, con filtro opcional por servicio. Disponible para todos los roles autenticados"),
		func(ctx context.Context, _ *mcp.CallToolRequest, in searchProfessionalsAdvancedIn) (*mcp.CallToolResult, dto.SearchProfessionalsAdvancedResult, error) {
			caller, err := auth.RequireCaller(ctx)
			if err != nil {
				return nil, dto.SearchProfessionalsAdvancedResult{}, s.toMCPError(err)
			}
			result, err := s.cfg.SearchProfessionalsAdvanced.Execute(ctx, dto.SearchProfessionalsAdvancedInput{
				Caller:    *caller,
				QueryText: strings.TrimSpace(in.QueryText),
				ServiceID: strings.TrimSpace(in.ServiceID),
			})
			if err != nil {
				return nil, dto.SearchProfessionalsAdvancedResult{}, s.toMCPError(err)
			}
			if result == nil {
				return nil, dto.SearchProfessionalsAdvancedResult{Results: []dto.ProfessionalSearchEntry{}}, nil
			}
			// Fail-closed wire contract: the results array is always an array, never
			// null, so Hermes can iterate it unconditionally.
			out := *result
			if out.Results == nil {
				out.Results = []dto.ProfessionalSearchEntry{}
			}
			return nil, out, nil
		})
	s.toolNames["search_professionals_advanced"] = struct{}{}
}

// getProfessionalIn is the input of get_professional. The field is OPTIONAL, the
// opposite of the agenda read below: an absent professional_id asks for the
// caller's own record, which is the whole point of the tool for a staff caller.
// The use case resolves the self case through auth.AuthorizeProfessionalRead, so
// the transport never inspects the role to decide it.
type getProfessionalIn struct {
	ProfessionalID string `json:"professional_id,omitempty"`
}

// professionalDetailOut is the record projection every admitted caller receives
// (feat-staff-self-read S4): the counterpart of professionalOut with the contact
// fields REMOVED — they are not a conditional pair inside this object because
// they must be omittable independently of the identity.
type professionalDetailOut struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RoleSpecialty *string  `json:"role_specialty,omitempty"`
	Status        string   `json:"status"`
	Specialties   []string `json:"specialties"`
}

// getProfessionalOut is the pinned output contract of get_professional: the
// record plus the contact fields at the top level, present ONLY when the use case
// decided the caller may see them (owner or the professional itself). omitempty
// is load-bearing: a nil pointer renders as an ABSENT key, so Hermes can tell
// "withheld" from "stored empty" — the same mold as professionalOut.
type getProfessionalOut struct {
	Professional professionalDetailOut `json:"professional"`
	Email        *string               `json:"email,omitempty"`
	Phone        *string               `json:"phone,omitempty"`
}

// registerGetProfessionalTool wires the single-record read (S1).
func (s *Server) registerGetProfessionalTool() {
	if s.cfg.GetProfessional == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("get_professional",
		"Devuelve el registro de un profesional (identidad, estado y especialidades). Sin professional_id devuelve el registro propio. email y teléfono solo se incluyen para el owner y para el propio profesional. Disponible para owner, admin y staff"),
		func(ctx context.Context, _ *mcp.CallToolRequest, in getProfessionalIn) (*mcp.CallToolResult, getProfessionalOut, error) {
			caller, err := auth.RequireCaller(ctx)
			if err != nil {
				return nil, getProfessionalOut{}, s.toMCPError(err)
			}
			result, err := s.cfg.GetProfessional.Execute(ctx, dto.GetProfessionalInput{
				Caller:         *caller,
				ProfessionalID: strings.TrimSpace(in.ProfessionalID),
			})
			if err != nil {
				return nil, getProfessionalOut{}, s.toMCPError(err)
			}
			// Fail closed: a success without a record would render a zero-value
			// professional, which is a well-formed lie (same guard class as
			// errNilMaintenanceEntity).
			if result == nil {
				return nil, getProfessionalOut{}, s.toMCPError(errNilMaintenanceEntity)
			}
			specialties := result.Professional.Specialties
			if specialties == nil {
				// The wire always carries an array, never null, so Hermes can iterate
				// it unconditionally.
				specialties = []string{}
			}
			return nil, getProfessionalOut{
				Professional: professionalDetailOut{
					ID:            result.Professional.ID,
					Name:          result.Professional.Name,
					RoleSpecialty: result.Professional.RoleSpecialty,
					Status:        result.Professional.Status,
					Specialties:   specialties,
				},
				Email: result.Email,
				Phone: result.Phone,
			}, nil
		})
	s.toolNames["get_professional"] = struct{}{}
}

// getProfessionalSchedulesIn is the input of get_professional_schedules. The
// field is required (no omitempty): an agenda read without a professional is
// always a client bug, not a valid request.
type getProfessionalSchedulesIn struct {
	ProfessionalID string `json:"professional_id"`
}

// professionalIdentityOut is the identity half of the agenda read output
// (design §8.3 D4): id, name and status only. email/phone never cross here even
// though the entity carries them — the gate needs to name the professional, not
// to expose the roster's contacts.
type professionalIdentityOut struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// professionalScheduleRow is the pinned FOUR-field agenda row of
// get_professional_schedules (issue #153): the response already scopes every
// row to professional.id at the top level, so per-row professional_id was
// redundant there and is not rendered. The upsert_schedule read-back keeps the
// full stored-row shape (scheduleOut) — its professional_id is not redundant
// within its own response.
type professionalScheduleRow struct {
	ID        int    `json:"id"`
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// getProfessionalSchedulesOut is the pinned output contract of
// get_professional_schedules: the identity of the one professional the read is
// scoped to, plus the weekly agenda ordered by day_of_week (0=Sunday..6).
type getProfessionalSchedulesOut struct {
	Professional professionalIdentityOut   `json:"professional"`
	Schedules    []professionalScheduleRow `json:"schedules"`
}

// registerGetProfessionalSchedulesTool wires the agenda read (S7b; roles widened
// to owner/admin/staff by S3).
func (s *Server) registerGetProfessionalSchedulesTool() {
	if s.cfg.GetProfessionalSchedules == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("get_professional_schedules",
		"Devuelve la agenda semanal de un profesional (identidad + franjas ordenadas por día, 0=domingo..6=sábado). Disponible para owner, admin y staff; staff solo puede leer su propia agenda"),
		func(ctx context.Context, _ *mcp.CallToolRequest, in getProfessionalSchedulesIn) (*mcp.CallToolResult, getProfessionalSchedulesOut, error) {
			caller, err := auth.RequireCaller(ctx)
			if err != nil {
				return nil, getProfessionalSchedulesOut{}, s.toMCPError(err)
			}
			result, err := s.cfg.GetProfessionalSchedules.Execute(ctx, dto.GetProfessionalSchedulesInput{
				Caller:         *caller,
				ProfessionalID: strings.TrimSpace(in.ProfessionalID),
			})
			if err != nil {
				return nil, getProfessionalSchedulesOut{}, s.toMCPError(err)
			}
			// Fail closed: a port that answers success without a result is a bug, and
			// a zero-value identity plus an empty agenda would be a malformed
			// response the gate could mistake for real (same guard class as
			// errNilMaintenanceEntity; the list read above CAN answer empty, this
			// one cannot — "no result" is not "no agenda").
			if result == nil {
				return nil, getProfessionalSchedulesOut{}, s.toMCPError(errNilMaintenanceEntity)
			}
			out := getProfessionalSchedulesOut{
				Professional: professionalIdentityOut{
					ID:     result.Professional.ID,
					Name:   result.Professional.Name,
					Status: result.Professional.Status,
				},
				Schedules: make([]professionalScheduleRow, 0, len(result.Schedules)),
			}
			for _, row := range result.Schedules {
				// Skipping a nil row would answer with a silently incomplete
				// agenda — the exact failure mode this read exists to prevent
				// (the gate would verify a week that is not the stored one), so
				// it fails closed like a nil result.
				if row == nil {
					return nil, getProfessionalSchedulesOut{}, s.toMCPError(errNilMaintenanceEntity)
				}
				out.Schedules = append(out.Schedules, professionalScheduleRow{
					ID:        row.ID,
					DayOfWeek: row.DayOfWeek,
					StartTime: row.StartTime,
					EndTime:   row.EndTime,
				})
			}
			return nil, out, nil
		})
	s.toolNames["get_professional_schedules"] = struct{}{}
}

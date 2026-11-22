package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// findAvailableSlotsIn is the input of find_available_slots (REQ-MT-015, REQ-AV-3).
// Field names are the MCP wire contract; the dto carrier is wire-free, so this
// struct owns the shape.
//
// From/To stay plain strings: the range is a calendar window in the BUSINESS
// timezone, and parsing "2031-03-03" into time.Time here would have to invent a
// zone (an offset-bearing instant would make the day boundaries depend on the
// caller). The use case parses them and builds every candidate with
// time.Date(..., loc).
//
// ProfessionalID and Limit are pointers so "absent" is distinguishable from a
// zero value: an absent professional means "everyone who can serve this", and an
// absent limit means the default 10 (a limit of 0 is invalid, not absent).
type findAvailableSlotsIn struct {
	ServiceID      string  `json:"service_id"`
	ProfessionalID *string `json:"professional_id,omitempty"`
	From           string  `json:"from"`
	To             string  `json:"to"`
	Limit          *int    `json:"limit,omitempty"`
}

// registerSlotsTools wires the date-range slot search onto the SDK server
// (agent-surface T2, issue #241). The tool is "any authenticated": it has NO
// ToolRBAC entry in the composition root (same mold as check_availability), and
// the use case re-asserts auth.RequireAuthenticated.
func (s *Server) registerSlotsTools() {
	if s.cfg.FindAvailableSlots == nil {
		return
	}

	mcp.AddTool(s.impl, s.mcpTool("find_available_slots",
		"Busca turnos libres de un servicio en un rango de fechas inclusivo (máximo 14 días). Devuelve los slots disponibles ordenados por fecha/hora con el profesional de cada uno, más el total de candidatos y si la respuesta quedó truncada por el límite. A diferencia de check_availability, no informa el motivo de cada slot ocupado: solo lista los libres"),
		func(ctx context.Context, _ *mcp.CallToolRequest, in findAvailableSlotsIn) (*mcp.CallToolResult, dto.FindAvailableSlotsResult, error) {
			caller, err := auth.RequireCaller(ctx)
			if err != nil {
				return nil, dto.FindAvailableSlotsResult{}, s.toMCPError(err)
			}
			result, err := s.cfg.FindAvailableSlots.Execute(ctx, dto.FindAvailableSlotsInput{
				Caller:         *caller,
				ServiceID:      in.ServiceID,
				ProfessionalID: in.ProfessionalID,
				From:           in.From,
				To:             in.To,
				Limit:          in.Limit,
			})
			if err != nil {
				return nil, dto.FindAvailableSlotsResult{}, s.toMCPError(err)
			}
			return nil, *result, nil
		})
	s.toolNames["find_available_slots"] = struct{}{}
}

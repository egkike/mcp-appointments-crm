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
func (s *Server) registerProfessionalsReadTools() {
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

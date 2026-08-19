package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getBusinessProfileIn is the input of get_business_profile: the profile is a
// singleton, so the tool takes no arguments. The SDK schema for an empty
// struct is {type: object} with no properties, which validates any argument
// object.
type getBusinessProfileIn struct{}

// registerProfileTool wires the get_business_profile tool onto the SDK server
// when the port is non-nil (T-09). The profile is not tenant-scoped, so the
// handler only resolves the authenticated caller (fail-closed, REQ-MT-007)
// and passes no input to the use case; the role restriction (owner/admin/
// staff) is enforced at the RBAC transport layer, not here.
func (s *Server) registerProfileTool() {
	if s.cfg.GetBusinessProfile == nil {
		return
	}
	mcp.AddTool(s.impl, s.mcpTool("get_business_profile", "Obtiene el perfil del negocio (nombre, descripción, horario y zona horaria)"),
		func(ctx context.Context, _ *mcp.CallToolRequest, _ getBusinessProfileIn) (*mcp.CallToolResult, entity.BusinessProfile, error) {
			if _, err := auth.RequireCaller(ctx); err != nil {
				return nil, entity.BusinessProfile{}, toMCPError(err)
			}
			profile, err := s.cfg.GetBusinessProfile.Execute(ctx)
			if err != nil {
				return nil, entity.BusinessProfile{}, toMCPError(err)
			}
			return nil, *profile, nil
		})
	s.toolNames["get_business_profile"] = struct{}{}
}

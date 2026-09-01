package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getLoyaltyReportIn is the input of get_loyalty_report.
type getLoyaltyReportIn struct {
	Period string `json:"period,omitempty"`
	TopN   *int   `json:"top_n,omitempty"`
}

// registerLoyaltyTools wires the loyalty report tool onto the SDK server when
// the corresponding port is non-nil. The tool is restricted to owner and admin
// roles via ToolRBAC in the composition root because rows expose client phone
// numbers (PII).
func (s *Server) registerLoyaltyTools() {
	if s.cfg.GetLoyaltyReport != nil {
		mcp.AddTool(s.impl, s.mcpTool("get_loyalty_report", "Reporte de clientes más frecuentes en un período. Expone teléfono (PII). Solo disponible para owner y admin"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in getLoyaltyReportIn) (*mcp.CallToolResult, dto.GetLoyaltyReportResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.GetLoyaltyReportResult{}, toMCPError(err)
				}
				result, err := s.cfg.GetLoyaltyReport.Execute(ctx, dto.GetLoyaltyReportInput{
					Caller: *caller,
					Period: in.Period,
					TopN:   in.TopN,
				})
				if err != nil {
					return nil, dto.GetLoyaltyReportResult{}, toMCPError(err)
				}
				if result == nil {
					return nil, dto.GetLoyaltyReportResult{Results: []dto.LoyaltyReportEntry{}}, nil
				}
				return nil, *result, nil
			})
		s.toolNames["get_loyalty_report"] = struct{}{}
	}
}

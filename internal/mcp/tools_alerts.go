package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getPendingAlertsIn is the input of get_pending_alerts.
type getPendingAlertsIn struct{}

// markAlertAsSentIn is the input of mark_alert_as_sent.
type markAlertAsSentIn struct {
	AlertID int `json:"alert_id"`
}

// registerAlertTools wires the two alert lifecycle tools onto the SDK server
// when the corresponding port is non-nil. Both tools are restricted to owner
// and admin roles via ToolRBAC in the composition root.
func (s *Server) registerAlertTools() {
	if s.cfg.GetPendingAlerts != nil {
		mcp.AddTool(s.impl, s.mcpTool("get_pending_alerts", "Lista las alertas pendientes vencidas ordenadas de más antigua a más reciente. Solo disponible para owner y admin"),
			func(ctx context.Context, _ *mcp.CallToolRequest, _ getPendingAlertsIn) (*mcp.CallToolResult, dto.GetPendingAlertsResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.GetPendingAlertsResult{}, toMCPError(err)
				}
				result, err := s.cfg.GetPendingAlerts.Execute(ctx, dto.GetPendingAlertsInput{Caller: *caller})
				if err != nil {
					return nil, dto.GetPendingAlertsResult{}, toMCPError(err)
				}
				if result == nil {
					return nil, dto.GetPendingAlertsResult{Alerts: []dto.PendingAlertView{}}, nil
				}
				return nil, *result, nil
			})
		s.toolNames["get_pending_alerts"] = struct{}{}
	}

	if s.cfg.MarkAlertAsSent != nil {
		mcp.AddTool(s.impl, s.mcpTool("mark_alert_as_sent", "Marca una alerta pendiente como enviada. Solo disponible para owner y admin"),
			func(ctx context.Context, _ *mcp.CallToolRequest, in markAlertAsSentIn) (*mcp.CallToolResult, dto.MarkAlertAsSentResult, error) {
				caller, err := auth.RequireCaller(ctx)
				if err != nil {
					return nil, dto.MarkAlertAsSentResult{}, toMCPError(err)
				}
				if in.AlertID <= 0 {
					return nil, dto.MarkAlertAsSentResult{}, toMCPError(&domain.SemanticError{
						Code:    domain.ErrCodeInvalidInput,
						Message: "alert_id debe ser un entero positivo",
					})
				}
				result, err := s.cfg.MarkAlertAsSent.Execute(ctx, dto.MarkAlertAsSentInput{
					Caller:  *caller,
					AlertID: in.AlertID,
				})
				if err != nil {
					return nil, dto.MarkAlertAsSentResult{}, toMCPError(err)
				}
				return nil, *result, nil
			})
		s.toolNames["mark_alert_as_sent"] = struct{}{}
	}
}

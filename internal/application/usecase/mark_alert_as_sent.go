package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// MarkAlertAsSentUseCase marks a pending alert as sent.
// Access is restricted to owner and admin roles.
type MarkAlertAsSentUseCase struct {
	alerts domainrepo.PendingAlertsRepo
}

// NewMarkAlertAsSentUseCase constructs the use case.
func NewMarkAlertAsSentUseCase(alerts domainrepo.PendingAlertsRepo) *MarkAlertAsSentUseCase {
	return &MarkAlertAsSentUseCase{alerts: alerts}
}

// Execute marks the alert as sent. Only owner/admin callers are allowed.
func (uc *MarkAlertAsSentUseCase) Execute(ctx context.Context, input dto.MarkAlertAsSentInput) (*dto.MarkAlertAsSentResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	if _, err := auth.RequireRole(ctx, auth.RoleOwner, auth.RoleAdmin); err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	if err := uc.alerts.MarkAsSent(ctx, input.AlertID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "alerta no encontrada o no pendiente", Cause: err}
		}
		return nil, fmt.Errorf("mark_alert_as_sent: %w", err)
	}
	return &dto.MarkAlertAsSentResult{AlertID: input.AlertID, Status: "sent"}, nil
}

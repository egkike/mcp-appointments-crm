package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// GetPendingAlertsUseCase returns due pending alerts ordered oldest first.
// Access is restricted to owner and admin roles.
type GetPendingAlertsUseCase struct {
	alerts domainrepo.PendingAlertsRepo
	clock  Clock
}

// Clock abstracts time.Now() for testability.
type Clock interface {
	Now() time.Time
}

// systemClock is the real clock.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// NewGetPendingAlertsUseCase constructs the use case.
func NewGetPendingAlertsUseCase(alerts domainrepo.PendingAlertsRepo) *GetPendingAlertsUseCase {
	return &GetPendingAlertsUseCase{alerts: alerts, clock: systemClock{}}
}

// SetClock replaces the clock. It is intended for tests.
func (uc *GetPendingAlertsUseCase) SetClock(clock Clock) {
	uc.clock = clock
}

// Execute runs the use case. Only owner/admin callers are allowed.
func (uc *GetPendingAlertsUseCase) Execute(ctx context.Context, input dto.GetPendingAlertsInput) (*dto.GetPendingAlertsResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	if _, err := auth.RequireRole(ctx, auth.RoleOwner, auth.RoleAdmin); err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	pending, err := uc.alerts.FindPending(ctx, uc.clock.Now())
	if err != nil {
		return nil, fmt.Errorf("get_pending_alerts: %w", err)
	}

	alerts := make([]dto.PendingAlertView, 0, len(pending))
	for _, a := range pending {
		view := dto.PendingAlertView{
			AlertID:           a.ID,
			Type:              a.Type,
			Message:           a.Message,
			ScheduledDatetime: a.ScheduledDatetime.UTC().Format(time.RFC3339),
			RelatedBookingID:  a.RelatedBookingID,
			CreatedAt:         a.CreatedAt,
		}
		alerts = append(alerts, view)
	}
	return &dto.GetPendingAlertsResult{Alerts: alerts}, nil
}

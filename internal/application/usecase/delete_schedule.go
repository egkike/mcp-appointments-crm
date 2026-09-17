package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// DeleteScheduleUseCase removes a professional's weekly slot for a single day.
// Access is restricted to the owner role (ADR-0015 Decision 2).
type DeleteScheduleUseCase struct {
	schedules domainrepo.SchedulesRepo
	logger    *slog.Logger
}

// NewDeleteScheduleUseCase constructs the use case. A nil logger falls back to
// slog.Default() (see maintenanceLogger).
func NewDeleteScheduleUseCase(schedules domainrepo.SchedulesRepo, logger *slog.Logger) *DeleteScheduleUseCase {
	return &DeleteScheduleUseCase{schedules: schedules, logger: maintenanceLogger(logger)}
}

// Execute deletes the (professional, day) slot. Day-range validation stays in
// SchedulesRepo.Delete; this use case maps the missing-row sentinel to a message
// the LLM can act on.
func (uc *DeleteScheduleUseCase) Execute(ctx context.Context, input dto.DeleteScheduleInput) (*dto.DeleteScheduleResult, error) {
	ctx = auth.WithCaller(ctx, input.Caller)
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	if err := uc.schedules.Delete(ctx, input.ProfessionalID, input.DayOfWeek); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "el profesional no tiene un horario ese día", Cause: err}
		case errors.Is(err, domain.ErrInvalidInput):
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el día debe estar entre 0 (domingo) y 6 (sábado)",
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("delete_schedule: %w", err)
	}

	logMaintenanceMutation(uc.logger, caller, "delete_schedule", "schedule", maintenanceScheduleID(input.ProfessionalID, input.DayOfWeek), "delete")
	return &dto.DeleteScheduleResult{
		ProfessionalID: input.ProfessionalID,
		DayOfWeek:      input.DayOfWeek,
		Status:         "deleted",
	}, nil
}

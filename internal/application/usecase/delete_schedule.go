package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
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
	// B8 f1: pass auth.RequireRole's own error through unchanged. The old
	// wrapper collapsed RequireRole's ErrCodeUnauthenticated (missing caller)
	// into ErrCodeForbidden, erasing the 401/403 distinction this layer's
	// contract documents (internal/auth/helpers.go). The MCP middleware gates
	// non-owners before the handler, so clients see no difference today; this
	// keeps the defense-in-depth seam honest.
	caller, err := auth.RequireRole(ctx, auth.RoleOwner)
	if err != nil {
		var sem *domain.SemanticError
		if errors.As(err, &sem) {
			return nil, sem
		}
		return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
	}

	// DayOfWeek arrives as the wire int (0..6) and is converted to the nominal
	// schedules encoding at this boundary, so the repository never sees a raw
	// int that could carry the other (business_hours) convention.
	if err := uc.schedules.Delete(ctx, input.ProfessionalID, entity.ScheduleWeekday(input.DayOfWeek)); err != nil {
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
